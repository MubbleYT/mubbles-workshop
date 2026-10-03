import {readFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import path from 'node:path';
import {publishRelease} from './publish.mjs';
const directory=process.argv[2];
if(!directory) throw new Error('Supply the validated release directory.');
const token=process.env.GH_TOKEN;
if(!token) throw new Error('Publishing credential is unavailable.');
const metadata=JSON.parse(await readFile(path.join(directory,'release.json'),'utf8'));
const repo='MubbleYT/mubbles-workshop';
const tag=`${metadata.id}--v${metadata.version}`;
async function request(endpoint,method='GET',body,contentType='application/json') {
  const url=endpoint.startsWith('https:')?endpoint:`https://api.github.com/repos/${repo}${endpoint}`;
  const response=await fetch(url,{method,headers:{Authorization:`Bearer ${token}`,Accept:'application/vnd.github+json','X-GitHub-Api-Version':'2022-11-28',...(body?{'Content-Type':contentType}:{})},body,signal:AbortSignal.timeout(300000)});
  if(!response.ok){const error=new Error(`GitHub returned HTTP ${response.status}`);error.status=response.status;throw error;}
  return response.json();
}
let release;
try {release=await request(`/releases/tags/${encodeURIComponent(tag)}`);} catch(error) {
  if(error.status!==404)throw error;
  for(let page=1;!release;page++){
    const entries=await request(`/releases?per_page=100&page=${page}`);
    release=entries.find(r=>r.tag_name===tag);if(entries.length<100)break;
  }
}
if(!release)release=await request('/releases','POST',JSON.stringify({tag_name:tag,target_commitish:'main',name:`${metadata.title} ${metadata.version}`,body:`Mubble's Workshop release: ${metadata.id}\n\n${metadata.changelog}`,draft:true,prerelease:false}));
if(!release.body?.startsWith(`Mubble's Workshop release: ${metadata.id}\n`))throw new Error('Release tag belongs to a different project.');
const sourceName=`mubbles-${metadata.id}-${metadata.version}-source.zip`;
const bytes=await readFile(path.join(directory,sourceName));
const expected=createHash('sha256').update(bytes).digest('hex');
let asset=(release.assets||[]).find(a=>a.name===sourceName);
if(!asset){
  if(!release.draft)throw new Error('Published release is missing its source; refusing to alter it.');
  asset=await request(`https://uploads.github.com/repos/${repo}/releases/${release.id}/assets?name=${encodeURIComponent(sourceName)}`,'POST',bytes,'application/zip');
}
if(asset.state!=='uploaded'||asset.size!==bytes.length||asset.digest!==`sha256:${expected}`)throw new Error('Source upload checksum verification failed.');
const result=await publishRelease({file:path.join(directory,`mubbles-${metadata.id}-${metadata.version}.jar`),metadata,token});
console.log(JSON.stringify(result));
