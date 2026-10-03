import {readFile, open, stat} from 'node:fs/promises';
import {createReadStream} from 'node:fs';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {createInterface} from 'node:readline/promises';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
import {REPOSITORY, CATALOG_VERSION, validateMetadata, validateReleaseInput, validateCatalog} from '../dist/catalog.mjs';

const API_VERSION = '2026-03-10';
const catalogPath = 'dist/catalog.json';
class APIError extends Error {
  constructor(status) { super(`GitHub returned HTTP ${status}.`); this.status = status; }
}
async function fileDigest(filename) {
  const hash = createHash('sha256');
  for await (const chunk of createReadStream(filename)) hash.update(chunk);
  return hash.digest('hex');
}
export async function prepareRelease(file, input) {
  if (input.rightsConfirmed !== true) throw new Error('Confirm that you have permission to distribute the file: rightsConfirmed must be true.');
  const project = validateMetadata(input);
  if (project.source.includes('REPLACE-WITH-')) throw new Error('Replace the example source URL with the actual source repository.');
  const info = validateReleaseInput(input);
  const filename = path.basename(file);
  if (filename.length > 200 || /[\\/\x00-\x1f]/.test(filename)) throw new Error('Use a simple release filename up to 200 characters.');
  const extension = project.type === 'Mods' ? '.jar' : project.type === 'Modpacks' ? '.mrpack' : '.zip';
  if (!filename.toLowerCase().endsWith(extension)) throw new Error(`This project requires a ${extension} file.`);
  const details = await stat(file);
  if (!details.isFile() || details.size < 4 || details.size >= 2 * 1024 ** 3) throw new Error('Choose an archive smaller than 2 GiB.');
  const handle = await open(file, 'r');
  const bytes = Buffer.alloc(4);
  try { await handle.read(bytes, 0, 4, 0); } finally { await handle.close(); }
  if (bytes[0] !== 80 || bytes[1] !== 75 || !['0304', '0506', '0708'].includes(bytes.subarray(2).toString('hex'))) throw new Error('The file does not have a ZIP/JAR archive signature.');
  return {project, release: {...info, filename, size: details.size, sha256: await fileDigest(file), tag: `${project.id}--v${info.version}`}};
}
export function mergeCatalog(catalog, project, release) {
  const current = validateCatalog(catalog);
  const previous = current.projects.find(p => p.id === project.id);
  const oldVersion = previous?.releases.find(r => r.version === release.version);
  if (oldVersion && (oldVersion.sha256 !== release.sha256 || oldVersion.filename !== release.filename)) throw new Error('That version already has a different file. Increase the version number.');
  const releases = previous?.releases.filter(r => r.version !== release.version) || [];
  const next = {...current, projects: [...current.projects.filter(p => p.id !== project.id), {...project, releases: [release, ...releases]}]};
  next.projects.sort((a,b) => a.id.localeCompare(b.id));
  const normalized = validateCatalog(next);
  if (JSON.stringify(normalized.projects) === JSON.stringify(current.projects)) return current;
  return {...normalized, updatedAt: new Date().toISOString()};
}
export async function publishRelease({file, metadata, token, fetchImpl = fetch}) {
  const prepared = await prepareRelease(file, metadata);
  if (!token) throw new Error('Sign in using GitHub CLI, or supply GH_TOKEN in the publishing environment.');
  const request = async (endpoint, {method = 'GET', json, body, contentType, length, octet = false} = {}) => {
    const url = endpoint.startsWith('https:') ? new URL(endpoint) : new URL(`https://api.github.com/repos/${REPOSITORY}${endpoint}`);
    if (!['api.github.com','uploads.github.com'].includes(url.hostname) || url.protocol !== 'https:') throw new Error('Unexpected GitHub API host.');
    const headers = {Authorization: `Bearer ${token}`, Accept: octet ? 'application/octet-stream' : 'application/vnd.github+json', 'X-GitHub-Api-Version': API_VERSION};
    if (json !== undefined) { headers['Content-Type'] = 'application/json'; body = JSON.stringify(json); }
    if (contentType) headers['Content-Type'] = contentType;
    if (length !== undefined) headers['Content-Length'] = String(length);
    const response = await fetchImpl(url.href, {method, headers, body, ...(body && typeof body !== 'string' && !Buffer.isBuffer(body) ? {duplex: 'half'} : {}), signal: AbortSignal.timeout(300000)});
    if (!response.ok) throw new APIError(response.status);
    return octet ? Buffer.from(await response.arrayBuffer()) : response.status === 204 ? null : response.json();
  };
  const repo = await request('');
  if (repo.private) throw new Error('The free public Workshop needs a public release repository.');
  const branch = repo.default_branch;
  if (!branch) throw new Error('Upload the Workshop source to the repository first.');
  const readCatalog = async () => {
    let entry;
    try { entry = await request(`/contents/${catalogPath}?ref=${encodeURIComponent(branch)}`); }
    catch (e) { if (e.status === 404) throw new Error('Upload the Workshop source (including dist/catalog.json) to GitHub before publishing a mod.'); throw e; }
    if (entry.encoding !== 'base64' || typeof entry.content !== 'string' || !entry.sha) throw new Error('Could not read the existing catalog.');
    return {sha: entry.sha, catalog: validateCatalog(JSON.parse(Buffer.from(entry.content,'base64').toString('utf8')))};
  };
  const initial = await readCatalog();
  const prior = initial.catalog.projects.find(p => p.id === prepared.project.id)?.releases.find(r => r.version === prepared.release.version);
  if (prior && (prior.sha256 !== prepared.release.sha256 || prior.filename !== prepared.release.filename)) throw new Error('That version already has a different file. Increase the version number.');
  let release;
  try { release = await request(`/releases/tags/${encodeURIComponent(prepared.release.tag)}`); }
  catch (e) {
    if (e.status !== 404) throw e;
    // Drafts may not appear in the by-tag endpoint. Find one left by an interrupted upload.
    for (let page=1; !release; page++) {
      const entries = await request(`/releases?per_page=100&page=${page}`);
      release = entries.find(r => r.tag_name === prepared.release.tag);
      if (entries.length < 100) break;
    }
  }
  if (!release) {
    release = await request('/releases', {method:'POST', json:{tag_name:prepared.release.tag, target_commitish:branch,
      name:`${prepared.project.title} ${prepared.release.version}`, body:`Mubble's Workshop release: ${prepared.project.id}\n\n${prepared.release.changelog}`, draft:true, prerelease:false}});
  }
  if (!release.body?.startsWith(`Mubble's Workshop release: ${prepared.project.id}\n`)) throw new Error('This release tag is already used by something else. Choose a different version.');
  const descriptor = JSON.stringify({schemaVersion:CATALOG_VERSION, ...prepared}, null, 2) + '\n';
  const assets = release.assets || [];
  let asset = assets.find(a => a.name === prepared.release.filename);
  const manifest = assets.find(a => a.name === 'workshop-release.json');
  if (manifest) {
    const existing = await request(`/releases/assets/${manifest.id}`, {octet:true});
    if (existing.toString('utf8') !== descriptor) throw new Error('That release already has different metadata. Use a new version.');
  }
  const verifyAsset = async candidate => {
    const digest = candidate.digest?.startsWith('sha256:') ? candidate.digest.slice(7) : createHash('sha256').update(await request(`/releases/assets/${candidate.id}`,{octet:true})).digest('hex');
    if (candidate.state !== 'uploaded' || candidate.size !== prepared.release.size || digest !== prepared.release.sha256) throw new Error('The uploaded release file differs or its upload is incomplete. Remove the incomplete draft manually or use a new version.');
  };
  if (asset) await verifyAsset(asset);
  if (!release.draft && (!asset || !manifest)) throw new Error('This published release is incomplete. Use a new version; published files will not be overwritten.');
  const upload = async (name, body, contentType) => request(`https://uploads.github.com/repos/${REPOSITORY}/releases/${release.id}/assets?name=${encodeURIComponent(name)}`,{method:'POST',body,contentType,length:Buffer.isBuffer(body)?body.length:prepared.release.size});
  if (!asset) {
    asset = await upload(prepared.release.filename, createReadStream(file), 'application/octet-stream');
    await verifyAsset(asset);
  }
  if (!manifest) await upload('workshop-release.json', Buffer.from(descriptor), 'application/json');
  if (release.draft) release = await request(`/releases/${release.id}`, {method:'PATCH',json:{draft:false,make_latest:'false'}});
  // Draft asset URLs can use a temporary tag. Refresh after publication.
  asset = release.assets.find(a => a.id === asset.id) || await request(`/releases/assets/${asset.id}`);
  const published = {...prepared.release, downloadUrl:asset.browser_download_url, publishedAt:release.published_at};
  let updated = false;
  // A file SHA guard prevents simultaneous publishers from losing each other's releases.
  for (let attempt=0; attempt<4; attempt++) {
    const state = attempt === 0 ? initial : await readCatalog();
    const next = mergeCatalog(state.catalog, prepared.project, published);
    if (JSON.stringify(next) === JSON.stringify(state.catalog)) break;
    try {
      await request(`/contents/${catalogPath}`, {method:'PUT',json:{message:`Publish ${prepared.project.id} ${published.version}`, branch, sha:state.sha, content:Buffer.from(JSON.stringify(next,null,2)+'\n').toString('base64')}});
      updated = true;
      break;
    } catch (e) {
      if (![409,422].includes(e.status) || attempt === 3) throw new Error(`The release was uploaded, but the catalog update failed. Run the same command again to finish. (${e.message})`);
    }
  }
  return {repository:REPOSITORY, tag:published.tag, downloadUrl:published.downloadUrl, sha256:published.sha256, catalogUpdated:updated};
}
function authToken() {
  if (process.env.GH_TOKEN || process.env.GITHUB_TOKEN) return process.env.GH_TOKEN || process.env.GITHUB_TOKEN;
  try { return execFileSync('gh',['auth','token','--hostname','github.com'],{encoding:'utf8',stdio:['ignore','pipe','pipe']}).trim(); }
  catch { throw new Error('Install GitHub CLI and run: gh auth login --hostname github.com --web\nDo not paste your token into a chat.'); }
}
async function wizard() {
  const rl = createInterface({input:process.stdin,output:process.stdout});
  try {
    console.log("Publish a finished release to Mubble's Workshop.\n");
    const ask = async (label, fallback='') => (await rl.question(`${label}${fallback ? ` [${fallback}]` : ''}: `)).trim() || fallback;
    const file = (await ask('Release file path (.jar, .zip or .mrpack)')).replace(/^"|"$/g,'');
    const manifest = (await ask('Release details JSON path (or press Enter to enter details)')).replace(/^"|"$/g,'');
    if (manifest) return {file,metadata:JSON.parse(await readFile(manifest,'utf8'))};
    const metadata = {};
    metadata.id = await ask('Project ID, e.g. water-erosion');
    metadata.title = await ask('Project title', "Mubble's ");
    metadata.author = 'Mubble';
    metadata.type = await ask('Type: Mods, Shaders, Resource packs or Modpacks','Mods');
    metadata.summary = await ask('One-line description');
    metadata.description = await ask('Description and installation instructions',metadata.summary);
    metadata.method = await ask('Creation method','Primarily AI-generated');
    metadata.tools = await ask('AI tools used','ChatGPT');
    metadata.source = await ask('Source code HTTPS URL');
    metadata.license = await ask('License','All rights reserved');
    metadata.version = await ask('Release version','1.0.0');
    metadata.mc = await ask('Minecraft version','26.3');
    metadata.loader = await ask('Loader / renderer',metadata.type==='Shaders'?'Iris':metadata.type==='Resource packs'?'Vanilla':'Fabric');
    metadata.changelog = await ask('What changed in this release?');
    metadata.rightsConfirmed = (await ask('You have permission to distribute this file? Type yes')).toLowerCase()==='yes';
    return {file,metadata};
  } finally { rl.close(); }
}
export async function main(args=process.argv.slice(2)) {
  if (args.includes('--help')) {
    console.log('node tools/publish.mjs --file path/to/mod.jar --metadata path/to/release.json [--dry-run]\nWithout arguments, opens a publishing wizard. Authentication: GitHub CLI login or GH_TOKEN.');
    return;
  }
  let input;
  if (!args.length) input = await wizard();
  else {
    const value = flag => {const i=args.indexOf(flag);return i<0?undefined:args[i+1]};
    for(let i=0;i<args.length;i++) {
      if (args[i]==='--dry-run') continue;
      if (!['--file','--metadata'].includes(args[i]) || !args[i+1] || args[i+1].startsWith('--')) throw new Error('Use --file and --metadata, with --dry-run for an offline check.');
      i++;
    }
    if (!value('--file') || !value('--metadata')) throw new Error('Both --file and --metadata are required.');
    input={file:value('--file'),metadata:JSON.parse(await readFile(value('--metadata'),'utf8'))};
  }
  if (args.includes('--dry-run')) {
    const prepared = await prepareRelease(input.file,input.metadata);
    console.log(JSON.stringify({dryRun:true,repository:REPOSITORY,...prepared},null,2));
    return;
  }
  const result = await publishRelease({...input,token:authToken()});
  console.log(`Published ${result.tag}\nDownload: ${result.downloadUrl}\nSHA-256: ${result.sha256}\n${result.catalogUpdated?'Catalog updated. The website will refresh after its deployment finishes.':'Catalog already contains this release.'}`);
}
if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch(error=>{console.error(error.message);process.exitCode=1});
}

