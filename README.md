# Packer Vagrant Box Google Cloud Storage (GCS)

[![GitHub latest release](https://img.shields.io/github/release/arnaud-dezandee/packer-vagrant-box-gcs.svg)](https://github.com/arnaud-dezandee/packer-vagrant-box-gcs/releases)
[![GoDoc](https://godoc.org/github.com/arnaud-dezandee/packer-vagrant-box-gcs?status.svg)](https://godoc.org/github.com/arnaud-dezandee/packer-vagrant-box-gcs)
[![Go Report Card](https://goreportcard.com/badge/github.com/arnaud-dezandee/packer-vagrant-box-gcs)](https://goreportcard.com/report/github.com/arnaud-dezandee/packer-vagrant-box-gcs)

This is a [Packer](https://www.packer.io) post-processor plugin to upload versioned boxes to
[Google Compute Storage](http://cloud.google.com/storage/) (GCS).

## Requirements
* [Packer](https://www.packer.io/intro/getting-started/install.html)
* [Go 1.13](https://golang.org/doc/install)

## Installation

### Install from release:

* Download binaries from the [releases page](https://github.com/arnaud-dezandee/packer-vagrant-box-gcs/releases).
* [Install](https://www.packer.io/docs/extending/plugins.html#installing-plugins) the plugin, or simply put it into the same directory with JSON templates.
* Move the downloaded binary to `~/.packer.d/plugins/`

### Install from sources:

Clone repository and build

```sh
$ mkdir -p $GOPATH/src/github.com/arnaud-dezandee; cd $GOPATH/src/github.com/arnaud-dezandee
$ git clone git@github.com:arnaud-dezandee/packer-vagrant-box-gcs.git
```
```sh
$ cd $GOPATH/src/github.com/arnaud-dezandee/packer-vagrant-box-gcs
$ go install
```

Link the build to Packer

```sh
$ ln -s $GOPATH/bin/packer-vagrant-box-gcs ~/.packer.d/plugins/packer-post-processor-vagrant-box-gcs 
```

## Usage

Add the plugin to your packer template after `vagrant` post-processor

```json
{
  "builder": [{
    "type": "virtualbox-iso"
  }],
  "post-processors": [
    [
      {
        "type": "vagrant"
      },
      {
        "type": "vagrant-box-gcs",
        "box_name": "myorg/mybox",
        "bucket": "my-gcs-bucket",
        "version": "1.0.0"
      }
    ]
  ]
}
```

This will create two objects inside the bucket
```
gs://my-gcs-bucket/myorg/mybox
gs://my-gcs-bucket/myorg/boxes/mybox/1.0.0/virtualbox.box
```

With the help of [vagrant-box-gcs](https://github.com/arnaud-dezandee/vagrant-box-gcs) plugin, you can now point your `Vagrantfile` to the manifest

```ruby
Vagrant.configure(2) do |config|
  config.vm.box = "myorg/mybox"
  config.vm.box_url = "gs://my-gcs-bucket/myorg/mybox"
end
```

## Authentication

Authenticating with Google Cloud services requires at most one JSON file.
Packer will look for credentials in the following places, preferring the first location found:

1.  An `account_file` option in your packer file.

2.  A JSON file (Service Account) whose path is specified by the
    `GOOGLE_APPLICATION_CREDENTIALS` environment variable.

3.  A JSON file in a location known to the `gcloud` command-line tool.
    (`gcloud auth application-default login` creates it)

    On Windows, this is:

        %APPDATA%/gcloud/application_default_credentials.json

    On other systems:

        $HOME/.config/gcloud/application_default_credentials.json

4.  On Google Compute Engine and Google App Engine Managed VMs, it fetches
    credentials from the metadata server. (Needs a correct VM authentication
    scope configuration)

## Configuration Reference

There are many configuration options available for the plugin. They are
segmented below into two categories: required and optional parameters.

### Required:

-   `box_name` (string) - The name of your box. (e.g. `hashicorp/precise64`)

-   `bucket` (string) - The GCS bucket name where files will be uploaded to.

-   `version` (string) - The version of the box you are uploading. (e.g. `1.0.0`)

### Optional:

-   `account_file` (string) - The JSON file containing your account credentials.

-   `box_dir` (string) - The path to a directory in your bucket to store boxes.

    Defaults to `{{ box_name[org] }}/boxes/{{ box_name[title] }}/{{ version }}`.

-   `box_manifest` (string) - The path to the manifest file in your bucket.

    Defaults to `{{ box_name }}`.

## Related

- [vagrant-box-gcs](https://github.com/arnaud-dezandee/vagrant-box-gcs) - Vagrant plugin to download boxes from Google GCS.
- [packer-post-processor-vagrant-s3](https://github.com/lmars/packer-post-processor-vagrant-s3) - A Packer post-processor to upload vagrant boxes to S3.
