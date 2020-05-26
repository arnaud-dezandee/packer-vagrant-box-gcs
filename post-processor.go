//go:generate mapstructure-to-hcl2 -type Config

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/storage/v1"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer/builder/googlecompute"
	"github.com/hashicorp/packer/common"
	"github.com/hashicorp/packer/helper/config"
	"github.com/hashicorp/packer/packer"
	"github.com/hashicorp/packer/post-processor/vagrant"
	"github.com/hashicorp/packer/template/interpolate"
)

var builderToVagrantProvider = map[string]string{
	"aws":          "aws",
	"azure":        "azure",
	"digitalocean": "digital_ocean",
	"docker":       "docker",
	"google":       "google",
	"hyperv":       "hyperv",
	"libvirt":      "libvirt",
	"lxc":          "lxc",
	"parallels":    "parallels",
	"scaleway":     "scaleway",
	"virtualbox":   "virtualbox",
	"vmware":       "vmware_desktop",
}

type Config struct {
	common.PackerConfig `mapstructure:",squash"`

	AccountFile string `mapstructure:"account_file"`

	BoxDir      string `mapstructure:"box_dir"`
	BoxName     string `mapstructure:"box_name"`
	BoxManifest string `mapstructure:"box_manifest"`
	Bucket      string `mapstructure:"bucket"`
	Version     string `mapstructure:"version"`

	BoxTitle string
	BoxOrg   string
	account  *jwt.Config
	ctx      interpolate.Context
}

type PostProcessor struct {
	config Config
}

func (p *PostProcessor) ConfigSpec() hcldec.ObjectSpec { return p.config.FlatMapstructure().HCL2Spec() }

func (p *PostProcessor) Configure(raws ...interface{}) error {
	err := config.Decode(&p.config, &config.DecodeOpts{
		Interpolate:        true,
		InterpolateContext: &p.config.ctx,
		InterpolateFilter:  &interpolate.RenderFilter{},
	}, raws...)
	if err != nil {
		return err
	}

	errs := new(packer.MultiError)

	var splits = strings.Split(p.config.BoxName, "/")
	p.config.BoxOrg = splits[0]
	p.config.BoxTitle = splits[1]

	// Set defaults
	if p.config.BoxDir == "" {
		p.config.BoxDir = path.Join(p.config.BoxOrg, "boxes", p.config.BoxTitle, p.config.Version)
	}
	if p.config.BoxManifest == "" {
		p.config.BoxManifest = p.config.BoxName
	}

	if p.config.AccountFile != "" {
		cfg, err := googlecompute.ProcessAccountFile(p.config.AccountFile)
		if err != nil {
			errs = packer.MultiErrorAppend(errs, err)
		}
		p.config.account = cfg
	}

	templates := map[string]*string{
		"box_name": &p.config.BoxName,
		"bucket":   &p.config.Bucket,
		"version":  &p.config.Version,
	}
	for key, ptr := range templates {
		if *ptr == "" {
			errs = packer.MultiErrorAppend(
				errs, fmt.Errorf("%s must be set", key))
		}
	}

	if len(errs.Errors) > 0 {
		return errs
	}

	return nil
}

func (p *PostProcessor) PostProcess(ctx context.Context, ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, bool, error) {
	client, err := googlecompute.NewClientGCE(p.config.account, "")
	if err != nil {
		return nil, false, false, err
	}
	service, err := storage.New(client)
	if err != nil {
		return nil, false, false, err
	}

	// Only accept input from the vagrant post-processor
	if artifact.BuilderId() != vagrant.BuilderId {
		err := fmt.Errorf("Unknown artifact type: %s\nCan only upload box from vagrant post-processor", artifact.BuilderId())
		return nil, false, false, err
	}

	// Assume there is only one .box file to upload
	box := artifact.Files()[0]
	if !strings.HasSuffix(box, ".box") {
		return nil, false, false, fmt.Errorf("Unknown artifact file from vagrant post-processor: %s", artifact.Files())
	}

	provider, ok := builderToVagrantProvider[artifact.Id()]
	if !ok {
		return nil, false, false, fmt.Errorf("Unknown artifact type, can't build box: %s", artifact.Id())
	}

	// Determine box params
	boxGcsKey := path.Join(p.config.BoxDir, fmt.Sprintf("%s.box", provider))
	boxStat, err := os.Stat(box)
	if err != nil {
		return nil, false, false, err
	}
	ui.Message(fmt.Sprintf("Box file: %s (%d bytes)", box, boxStat.Size()))
	boxChecksum, err := Sum256(box)
	if err != nil {
		return nil, false, false, err
	}
	ui.Message(fmt.Sprintf("Box sha256: %s", boxChecksum))

	// Upload box file
	boxFile, err := os.Open(box)
	if err != nil {
		err := fmt.Errorf("error opening %v", box)
		return nil, false, false, err
	}
	boxGcsUrl, err := UploadToBucket(service, ui, p.config.Bucket, boxGcsKey, boxFile, "application/x-gzip")
	if err != nil {
		return nil, false, false, err
	}

	// Update Manifest
	manifest, err := GetManifest(service, ui, p.config.Bucket, p.config.BoxManifest, p.config.BoxName)
	if err != nil {
		return nil, false, false, err
	}
	ui.Message(fmt.Sprintf("Manifest add: %s (%s)", provider, p.config.Version))
	if err := manifest.add(p.config.Version, &Provider{
		Name:         provider,
		Url:          boxGcsUrl,
		ChecksumType: "sha256",
		Checksum:     boxChecksum,
	}); err != nil {
		return nil, false, false, err
	}

	// Upload manifest file
	manifestFile, err := manifest.NewReader()
	if err != nil {
		return nil, false, false, err
	}
	manifestGcsUrl, err := UploadToBucket(service, ui, p.config.Bucket, p.config.BoxManifest, manifestFile, "application/json")
	if err != nil {
		return nil, false, false, err
	}

	return &Artifact{paths: []string{manifestGcsUrl, boxGcsUrl}}, false, false, nil
}

func UploadToBucket(service *storage.Service, ui packer.Ui, bucket string, object string, source io.Reader, ct string) (string, error) {
	ui.Say(fmt.Sprintf("Uploading %v: gs://%v/%v...", ct, bucket, object))

	storageObject, err := service.Objects.Insert(bucket, &storage.Object{Name: object, ContentType: ct}).Media(source).Do()
	if err != nil {
		ui.Say(fmt.Sprintf("Failed to upload: %v", storageObject))
		return "", err
	}

	return fmt.Sprintf("gs://%s/%s", storageObject.Bucket, storageObject.Name), nil
}

func GetManifest(service *storage.Service, ui packer.Ui, bucket string, object string, name string) (*Manifest, error) {
	storageObject, err := service.Objects.Get(bucket, object).Download()
	if err != nil {
		var gErr *googleapi.Error
		if errors.As(err, &gErr) {
			if gErr.Code == http.StatusNotFound {
				ui.Say(fmt.Sprintf("Manifest create: %s", name))
				return &Manifest{Name: name}, nil
			}
		}
		return nil, err
	}

	defer storageObject.Body.Close()

	manifest := &Manifest{}
	if err := json.NewDecoder(storageObject.Body).Decode(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func Sum256(filePath string) (string, error) {
	// open the file for reading
	file, err := os.Open(filePath)

	if err != nil {
		return "", err
	}

	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
