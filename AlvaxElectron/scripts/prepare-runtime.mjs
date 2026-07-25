import { access, cp, mkdir, rm } from 'node:fs/promises';
import { spawn } from 'node:child_process';
import path from 'node:path';
import process from 'node:process';
import console from 'node:console';

const PI_VERSION = '0.80.7';
const projectRoot = path.resolve(import.meta.dirname, '..');
const targetRoot = path.join(projectRoot, '.runtime', `${process.platform}-${process.arch}`);
const piRoot = path.join(targetRoot, 'pi');
const bundledAiConfigSource = process.env.ALVAX_BUNDLED_AI_CONFIG_FILE;

await rm(path.join(projectRoot, '.runtime'), { recursive: true, force: true });
await mkdir(targetRoot, { recursive: true });
if (bundledAiConfigSource) {
  await access(bundledAiConfigSource);
  await cp(bundledAiConfigSource, path.join(targetRoot, 'ai-runtime.json'));
}

await run(process.execPath, [
  path.join(resolveNpmRoot(), 'bin', 'npm-cli.js'),
  'install', `@earendil-works/pi-coding-agent@${PI_VERSION}`,
  '--prefix', piRoot, '--omit=dev', '--no-audit', '--no-fund', '--package-lock=false',
]);

const piCli = path.join(piRoot, 'node_modules', '@earendil-works', 'pi-coding-agent', 'dist', 'cli.js');
await access(piCli);
console.log(`Bundled Pi ${PI_VERSION} for ${process.platform}-${process.arch}; Node.js is provided by the host system.`);

function resolveNpmRoot() {
  const prefix = process.platform === 'win32' ? path.dirname(process.execPath) : path.dirname(path.dirname(process.execPath));
  return process.platform === 'win32' ? path.join(prefix, 'node_modules', 'npm') : path.join(prefix, 'lib', 'node_modules', 'npm');
}

function run(command, args) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd: projectRoot, stdio: 'inherit' });
    child.once('error', reject);
    child.once('exit', (code) => code === 0 ? resolve() : reject(new Error(`Command failed with exit code ${code}`)));
  });
}
