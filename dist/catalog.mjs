export const REPOSITORY = 'MubbleYT/mubbles-workshop';
export const CATALOG_VERSION = 1;
export const TYPES = ['Mods', 'Shaders', 'Resource packs', 'Modpacks'];
export const METHODS = ['Human-made', 'AI-assisted', 'Primarily AI-generated', 'Fully AI-generated'];
const slug = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const version = /^[A-Za-z0-9][A-Za-z0-9._+-]{0,39}$/;

function required(value, name, max = 10000) {
  if (typeof value !== 'string' || !value.trim() || value.length > max) throw new Error(`Invalid ${name}.`);
  return value.trim();
}
function list(value, name) {
  if (value === undefined) return [];
  if (!Array.isArray(value) || value.length > 50 || value.some(v => typeof v !== 'string' || v.length > 200)) throw new Error(`Invalid ${name}.`);
  return value.map(v => v.trim()).filter(Boolean);
}
export function httpsURL(value, name) {
  const url = new URL(required(value, name, 2000));
  if (url.protocol !== 'https:' || url.username || url.password) throw new Error(`Invalid ${name}.`);
  return url.href;
}
export function releaseURL(value) {
  const url = new URL(httpsURL(value, 'download URL'));
  if (url.hostname !== 'github.com' || !url.pathname.startsWith(`/${REPOSITORY}/releases/download/`)) throw new Error('Download must come from the Workshop release repository.');
  return url.href;
}
export function validateMetadata(input) {
  if (!input || typeof input !== 'object' || Array.isArray(input)) throw new Error('Project metadata must be an object.');
  const id = required(input.id, 'project ID', 80);
  if (!slug.test(id) || id.startsWith('local-')) throw new Error('Use a project ID such as water-erosion, without local-.');
  const type = required(input.type, 'project type', 30);
  const method = required(input.method, 'creation method', 40);
  if (!TYPES.includes(type) || !METHODS.includes(method)) throw new Error('Unknown project type or creation method.');
  const tools = typeof input.tools === 'string' ? input.tools.trim() : '';
  if (tools.length > 200 || (method !== 'Human-made' && !tools)) throw new Error('Disclose the AI tools used.');
  const source = input.source ? httpsURL(input.source, 'source URL') : '';
  if (type === 'Mods' && ['Primarily AI-generated', 'Fully AI-generated'].includes(method) && !source) throw new Error('Add a source URL for an AI-generated executable mod.');
  return {id, title: required(input.title, 'title', 100), author: required(input.author, 'author', 60), type,
    summary: required(input.summary, 'summary', 220), description: required(input.description, 'description'),
    method, tools, source, license: required(input.license, 'license', 200),
    tags: list(input.tags, 'tags'), features: list(input.features, 'features'),
    icon: ['shader', 'lights', 'erosion', 'nature', 'age', 'flicker'].includes(input.icon) ? input.icon : type === 'Shaders' ? 'shader' : type === 'Resource packs' ? 'nature' : 'lights'};
}
export function validateReleaseInput(input) {
  const v = required(input.version, 'version', 40);
  if (!version.test(v)) throw new Error('Version may contain letters, numbers, dots, underscores, + and -.');
  return {version: v, mc: required(input.mc, 'Minecraft version', 40), loader: required(input.loader, 'loader', 60),
    compat: list(input.compat, 'compatibility'), changelog: typeof input.changelog === 'string' ? input.changelog.slice(0, 5000) : ''};
}
export function validateRelease(input) {
  const info = validateReleaseInput(input);
  const filename = required(input.filename, 'filename', 200);
  if (/[\\/\x00-\x1f]/.test(filename) || !/\.(jar|zip|mrpack)$/i.test(filename)) throw new Error('Invalid release filename.');
  if (!Number.isSafeInteger(input.size) || input.size < 4 || input.size >= 2 * 1024 ** 3) throw new Error('Invalid release size.');
  if (!/^[a-f0-9]{64}$/.test(input.sha256)) throw new Error('Invalid SHA-256 checksum.');
  if (!Number.isFinite(Date.parse(input.publishedAt))) throw new Error('Invalid publication date.');
  return {...info, filename, size: input.size, sha256: input.sha256, downloadUrl: releaseURL(input.downloadUrl),
    tag: required(input.tag, 'release tag', 150), publishedAt: input.publishedAt};
}
export function validateCatalog(input) {
  if (!input || input.schemaVersion !== CATALOG_VERSION || input.repository !== REPOSITORY || !Array.isArray(input.projects)) throw new Error('Unsupported Workshop catalog.');
  const ids = new Set();
  const projects = input.projects.map(p => {
    const metadata = validateMetadata(p);
    if (ids.has(metadata.id)) throw new Error('Duplicate project ID.');
    ids.add(metadata.id);
    if (!Array.isArray(p.releases) || !p.releases.length) throw new Error('A public project needs a release.');
    const versions = new Set();
    const releases = p.releases.map(r => {
      const release = validateRelease(r);
      if (versions.has(release.version)) throw new Error('Duplicate project version.');
      versions.add(release.version);
      if (release.tag !== `${metadata.id}--v${release.version}`) throw new Error('Release tag does not match project.');
      const actual = decodeURIComponent(new URL(release.downloadUrl).pathname);
      const expected = `/${REPOSITORY}/releases/download/${release.tag}/${release.filename}`;
      if (actual !== expected) throw new Error('Release URL does not match its tag and file.');
      const extension = metadata.type === 'Mods' ? '.jar' : metadata.type === 'Modpacks' ? '.mrpack' : '.zip';
      if (!release.filename.toLowerCase().endsWith(extension)) throw new Error('Release file does not match project type.');
      return release;
    }).sort((a,b) => Date.parse(b.publishedAt) - Date.parse(a.publishedAt));
    return {...metadata, releases};
  });
  return {schemaVersion: CATALOG_VERSION, repository: REPOSITORY, updatedAt: input.updatedAt ?? null, projects};
}
export function displayProjects(catalog) {
  return validateCatalog(catalog).projects.map(p => ({...p, ...p.releases[0], sample: false, online: true,
    created: Date.parse(p.releases[0].publishedAt), downloads: 0}));
}
