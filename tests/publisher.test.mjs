import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,writeFile,rm,readFile} from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {prepareRelease,publishRelease,mergeCatalog} from '../tools/publish.mjs';
import {uploadSource} from '../tools/upload-source.mjs';
import {validateCatalog,displayProjects,REPOSITORY} from '../dist/catalog.mjs';

const metadata={id:'water-erosion',title:"Mubble's Water Erosion",author:'Mubble',type:'Mods',summary:'Flowing water erodes terrain.',description:'A test project.',method:'Primarily AI-generated',tools:'ChatGPT',source:'https://github.com/MubbleYT/water-erosion',license:'All rights reserved',version:'1.0.0',mc:'26.3',loader:'Fabric',compat:['Fabric API'],changelog:'Initial release.',rightsConfirmed:true};
const empty=()=>({schemaVersion:1,repository:REPOSITORY,updatedAt:null,projects:[]});
async function fixture(t){const dir=await mkdtemp(path.join(os.tmpdir(),'workshop-test-'));t.after(()=>rm(dir,{recursive:true,force:true}));const file=path.join(dir,'water-erosion.jar');await writeFile(file,Buffer.from([80,75,5,6,...Array(18).fill(0)]));return file}
function githubMock({conflict=false,interrupt=false}={}) {
  let catalog=empty(), sha='initial', releases=[], writes=0, next=1, conflictPending=conflict, interruptPending=interrupt;
  const binaries=new Map(), calls=[];
  const response=(status,data)=>new Response(Buffer.isBuffer(data)?data:JSON.stringify(data),{status});
  const fetchImpl=async(url,options={})=>{
    const u=new URL(url), method=options.method||'GET', base=`/repos/${REPOSITORY}`, p=u.pathname.slice(base.length);
    calls.push({method,path:p});
    const body=typeof options.body==='string'?JSON.parse(options.body):null;
    if(u.hostname==='api.github.com'&&p===''&&method==='GET')return response(200,{private:false,default_branch:'main'});
    if(p==='/contents/dist/catalog.json'&&method==='GET')return response(200,{sha,encoding:'base64',content:Buffer.from(JSON.stringify(catalog)).toString('base64')});
    if(p==='/contents/dist/catalog.json'&&method==='PUT'){
      if(conflictPending){conflictPending=false;sha='concurrent';const other={...catalog.projects[0],id:'other-mod',title:'Other mod'};const r={...other.releases[0],tag:'other-mod--v1.0.0',downloadUrl:`https://github.com/${REPOSITORY}/releases/download/other-mod--v1.0.0/other.jar`,filename:'other.jar'};other.releases=[r];catalog=mergeCatalog(catalog,other,r);return response(409,{})}
      if(body.sha!==sha)return response(409,{});
      catalog=JSON.parse(Buffer.from(body.content,'base64').toString('utf8'));sha='sha-'+(++writes);return response(200,{});
    }
    if(p.startsWith('/releases/tags/')){const release=releases.find(r=>r.tag_name===decodeURIComponent(p.slice('/releases/tags/'.length))&&!r.draft);return response(release?200:404,release||{})}
    if(p==='/releases'&&method==='GET')return response(200,releases);
    if(p==='/releases'&&method==='POST'){const r={...body,id:next++,assets:[],published_at:null};releases.push(r);return response(201,r)}
    if(u.hostname==='uploads.github.com'){
      const r=releases.find(r=>r.id===Number(p.split('/')[2])), name=u.searchParams.get('name');
      if(interruptPending&&name==='workshop-release.json'){interruptPending=false;return response(500,{})}
      const data=Buffer.isBuffer(options.body)?options.body:Buffer.concat(await Array.fromAsync(options.body));
      const a={id:next++,name,size:data.length,state:'uploaded',digest:'sha256:'+createHash('sha256').update(data).digest('hex'),browser_download_url:`https://github.com/${REPOSITORY}/releases/download/${encodeURIComponent(r.tag_name)}/${encodeURIComponent(name)}`};r.assets.push(a);binaries.set(a.id,data);return response(201,a);
    }
    if(p.startsWith('/releases/assets/')&&method==='GET')return response(200,binaries.get(Number(p.split('/')[3])));
    if(p.startsWith('/releases/')&&method==='PATCH'){const r=releases.find(r=>r.id===Number(p.split('/')[2]));Object.assign(r,body,{published_at:'2026-10-03T16:00:00Z'});return response(200,r)}
    throw new Error(`Unhandled mocked request ${method} ${p}`);
  };
  return {fetchImpl,calls,get catalog(){return catalog},get releases(){return releases},get writes(){return writes}};
}
test('offline validation computes the exact file checksum and rejects permission/extension errors',async t=>{
  const file=await fixture(t), p=await prepareRelease(file,metadata);
  assert.equal(p.release.sha256,createHash('sha256').update(await readFile(file)).digest('hex'));
  await assert.rejects(prepareRelease(file,{...metadata,rightsConfirmed:false}),/permission/);
  await assert.rejects(prepareRelease(file,{...metadata,type:'Shaders'}),/\.zip/);
  await assert.rejects(prepareRelease(file,{...metadata,source:''}),/source URL/);
});
test('publish uploads both assets before publishing and committing the catalog',async t=>{
  const file=await fixture(t), mock=githubMock();
  const result=await publishRelease({file,metadata,token:'fake-test-token',fetchImpl:mock.fetchImpl});
  assert.equal(result.catalogUpdated,true);assert.equal(mock.releases.length,1);assert.equal(mock.releases[0].assets.length,2);
  assert.equal(mock.releases[0].draft,false);assert.equal(displayProjects(mock.catalog)[0].online,true);
  assert.equal(mock.calls.at(-1).path,'/contents/dist/catalog.json');
  assert.equal(mock.calls.at(-2).method,'PATCH');
});
test('repeating the same completed release makes no additional uploads or catalog writes',async t=>{
  const file=await fixture(t), mock=githubMock(), input={file,metadata,token:'fake',fetchImpl:mock.fetchImpl};
  await publishRelease(input);const count=mock.calls.filter(c=>c.method==='POST').length;
  const result=await publishRelease(input);assert.equal(result.catalogUpdated,false);
  assert.equal(mock.calls.filter(c=>c.method==='POST').length,count);assert.equal(mock.writes,1);
});
test('interrupted upload resumes its draft without replacing the uploaded binary',async t=>{
  const file=await fixture(t), mock=githubMock({interrupt:true}), input={file,metadata,token:'fake',fetchImpl:mock.fetchImpl};
  await assert.rejects(publishRelease(input),/HTTP 500/);
  assert.equal(mock.releases[0].draft,true);assert.equal(mock.catalog.projects.length,0);
  await publishRelease(input);assert.equal(mock.releases.length,1);assert.equal(mock.releases[0].assets.length,2);
  assert.equal(mock.calls.filter(c=>c.path.endsWith('/assets')&&c.method==='POST').length,3);
});
test('changed binaries cannot overwrite an existing published version',async t=>{
  const file=await fixture(t), mock=githubMock(), input={file,metadata,token:'fake',fetchImpl:mock.fetchImpl};
  await publishRelease(input);await writeFile(file,Buffer.from([80,75,5,6,...Array(19).fill(1)]));
  await assert.rejects(publishRelease(input),/different file/);assert.equal(mock.releases[0].assets.length,2);
});
test('a new version keeps the older version downloadable',async t=>{
  const file=await fixture(t), mock=githubMock();await publishRelease({file,metadata,token:'fake',fetchImpl:mock.fetchImpl});
  await publishRelease({file,metadata:{...metadata,version:'1.1.0'},token:'fake',fetchImpl:mock.fetchImpl});
  assert.equal(mock.catalog.projects[0].releases.length,2);assert.equal(mock.releases.length,2);
});
test('SHA conflicts retry against the refreshed catalog and preserve concurrent releases',async t=>{
  const file=await fixture(t), mock=githubMock();await publishRelease({file,metadata,token:'fake',fetchImpl:mock.fetchImpl});
  const original=mock.fetchImpl;let conflict=true;
  const wrapped=async(url,options)=>{if(conflict&&options.method==='PUT'){conflict=false;await publishRelease({file,metadata:{...metadata,id:'nature-mod',title:'Nature mod'},token:'fake',fetchImpl:original})}return original(url,options)};
  await publishRelease({file,metadata:{...metadata,version:'1.1.0'},token:'fake',fetchImpl:wrapped});
  assert.equal(mock.catalog.projects.length,2);assert.equal(mock.catalog.projects.find(p=>p.id==='water-erosion').releases.length,2);
});
test('catalog validation rejects unrelated downloads, duplicate IDs, and unknown formats',async t=>{
  const file=await fixture(t), mock=githubMock();await publishRelease({file,metadata,token:'fake',fetchImpl:mock.fetchImpl});
  const bad=structuredClone(mock.catalog);bad.projects[0].releases[0].downloadUrl='https://example.com/mod.jar';
  assert.throws(()=>validateCatalog(bad),/Workshop release repository/);
  assert.throws(()=>validateCatalog({...mock.catalog,projects:[...mock.catalog.projects,...mock.catalog.projects]}),/Duplicate/);
  assert.throws(()=>validateCatalog({...mock.catalog,schemaVersion:99}),/Unsupported/);
});
test('source setup initializes an empty repository and creates a guarded commit',async()=>{
  const calls=[];let initialized=false;
  const fetchImpl=async(url,options)=>{
    const p=new URL(url).pathname.slice(`/repos/${REPOSITORY}`.length);calls.push({path:p,method:options.method,body:options.body?JSON.parse(options.body):null});
    const response=(status,data)=>new Response(JSON.stringify(data),{status});
    if(p==='')return response(200,{private:false,default_branch:'main'});
    if(p==='/git/ref/heads/main')return response(initialized?200:409,initialized?{object:{sha:'parent'}}:{});
    if(p==='/contents/README.md'){initialized=true;return response(201,{})}
    if(p==='/git/commits/parent')return response(200,{tree:{sha:'base'}});
    if(p==='/git/trees/base')return response(200,{tree:[{path:'README.md',type:'blob'}],truncated:false});
    if(p==='/git/trees')return response(201,{sha:'tree'});
    if(p==='/git/commits')return response(201,{sha:'commit'});
    if(p==='/git/refs/heads/main')return response(200,{});
    throw new Error(p);
  };
  await uploadSource({token:'fake',fetchImpl});
  assert.equal(calls.at(-1).body.force,false);assert.equal(calls.at(-1).body.sha,'commit');
  const tree=calls.find(c=>c.path==='/git/trees').body.tree;
  assert.ok(tree.some(f=>f.path==='dist/catalog.json'));assert.ok(tree.every(f=>!f.path.endsWith('.jar')));
});
test('source setup refuses to overwrite an initialized repository',async()=>{
  const mutations=[];
  const fetchImpl=async(url,options)=>{
    const p=new URL(url).pathname.slice(`/repos/${REPOSITORY}`.length);
    if(options.method!=='GET')mutations.push(p);
    const data=p===''?{private:false,default_branch:'main'}:p.startsWith('/git/ref/')?{object:{sha:'parent'}}:p==='/git/commits/parent'?{tree:{sha:'base'}}:{tree:[{path:'dist/catalog.json',type:'blob'}],truncated:false};
    return new Response(JSON.stringify(data),{status:200});
  };
  await assert.rejects(uploadSource({token:'fake',fetchImpl}),/already contains files/);assert.equal(mutations.length,0);
});
test('a server checksum mismatch leaves the release in draft and out of the catalog',async t=>{
  const file=await fixture(t),mock=githubMock();
  const fetchImpl=async(url,options)=>{
    const response=await mock.fetchImpl(url,options);
    if(new URL(url).hostname==='uploads.github.com'&&new URL(url).searchParams.get('name').endsWith('.jar')){
      const asset=await response.json();asset.digest='sha256:'+'0'.repeat(64);return new Response(JSON.stringify(asset),{status:201});
    }
    return response;
  };
  await assert.rejects(publishRelease({file,metadata,token:'fake',fetchImpl}),/differs/);
  assert.equal(mock.releases[0].draft,true);assert.equal(mock.catalog.projects.length,0);
});
