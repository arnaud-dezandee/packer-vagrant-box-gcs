/* eslint-disable no-await-in-loop, no-restricted-syntax */
const execa = require('execa');

const pkg = require('../package.json');

async function run(file, args, context) {
  const {
    cwd, env, stdout, stderr,
  } = context;
  const proc = execa(file, args, { cwd, env });
  proc.stdout.pipe(stdout, { end: false });
  proc.stderr.pipe(stderr, { end: false });
  return proc;
}

async function prepare(config, context) {
  const {
    nextRelease: { version }, logger,
  } = context;

  logger.log('Gox Cross Compilation');
  await run('gox', [
    '-arch', config.arch.join(' '),
    '-os', config.os.join(' '),
    '-output', `pkg/{{.OS}}-{{.Arch}}/${config.binary}`,
  ], context);

  logger.log('Archive creation');
  for (const arch of config.arch) {
    for (const os of config.os) {
      await run('zip', [
        '-j',
        `${pkg.name}_${version}_${os}_${arch}.zip`,
        `./pkg/${os}-${arch}/${config.binary}${os === 'windows' ? '.exe' : ''}`,
      ], context);
    }
  }
}

module.exports = {
  prepare,
};
