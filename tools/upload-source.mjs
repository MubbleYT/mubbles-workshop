import {readFile} from 'node:fs/promises';
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {REPOSITORY} from '../dist/catalog.mjs';

// This is only the first-time setup uploader, not a synchronization tool.
const files = ['.gitignore','package.json','README.md','START-WORKSHOP.cmd','start-windows.ps1','server.mjs',
  'LOGIN-GITHUB.cmd','UPLOAD-WORKSHOP.cmd','PUBLISH-RELEASE.cmd','examples/release.json',
  'dist/index.html','dist/style.css','dist/favicon.svg','dist/app.js','dist/catalog.mjs','dist/catalog.json','dist/_headers',
  'tools/publish.mjs','tools/upload-source.mjs','tests/publisher.test.mjs'];
export async function uploadSource({token, fetchImpl=fetch}={}) {
  const contents = await Promise.all(files.map(async filename=>({path:filename,mode:'100644',type:'blob',content:await readFile(new URL('../'+filename,import.meta.url),'utf8')})));
  token ||= process.env.GH_TOKEN || process.env.GITHUB_TOKEN;
  if (!token) {
    try { token=execFileSync('gh',['auth','token','--hostname','github.com'],{encoding:'utf8',stdio:['ignore','pipe','pipe']}).trim(); }
    catch { throw new Error('Run LOGIN-GITHUB.cmd first. GitHub CLI must be installed.'); }
  }
  async function api(endpoint, method='GET', body) {
    const response=await fetchImpl(`https://api.github.com/repos/${REPOSITORY}${endpoint}`,{method,
      headers:{Authorization:`Bearer ${token}`,Accept:'application/vnd.github+json','X-GitHub-Api-Version':'2026-03-10',...(body?{'Content-Type':'application/json'}:{})},
      ...(body?{body:JSON.stringify(body)}:{}),signal:AbortSignal.timeout(30000)});
    if(!response.ok){const error=new Error(`GitHub returned HTTP ${response.status}.`);error.status=response.status;throw error}
    return response.json();
  }
  const repo=await api('');
  if(repo.private)throw new Error('Workshop’s release repository must be public.');
  const branch=repo.default_branch || 'main';
  let ref;
  try {ref=await api(`/git/ref/heads/${encodeURIComponent(branch)}`)} catch(e){if(e.status!==404&&e.status!==409)throw e}
  if(!ref){
    console.log('Initializing the empty Workshop repository…');
    await api('/contents/README.md','PUT',{message:'Initialize Mubble’s Workshop',branch,content:Buffer.from(contents.find(f=>f.path==='README.md').content).toString('base64')});
    ref=await api(`/git/ref/heads/${encodeURIComponent(branch)}`);
  }
  const parent=await api(`/git/commits/${ref.object.sha}`);
  const existing=await api(`/git/trees/${parent.tree.sha}?recursive=1`);
  if(existing.truncated || existing.tree.some(f=>f.type==='blob' && f.path!=='README.md'))throw new Error('The repository already contains files. This setup uploader stops to avoid replacing them.');
  console.log('Uploading the Workshop source in one commit…');
  const tree=await api('/git/trees','POST',{base_tree:parent.tree.sha,tree:contents});
  const commit=await api('/git/commits','POST',{message:'Add Workshop live catalog and release publisher',tree:tree.sha,parents:[ref.object.sha]});
  await api(`/git/refs/heads/${encodeURIComponent(branch)}`,'PATCH',{sha:commit.sha,force:false});
  console.log(`Source uploaded to https://github.com/${REPOSITORY}\nNext: connect this repository to Cloudflare Pages, with output directory dist.`);
}
if(process.argv[1]===fileURLToPath(import.meta.url))uploadSource().catch(e=>{console.error(e.message);process.exitCode=1});
