"""Reproduce and test the authorized Weathering release from its pinned source input."""
import hashlib,json,os,pathlib,shutil,subprocess,sys,tarfile,urllib.request,zipfile
root=pathlib.Path(__file__).resolve().parent.parent
manifest=json.loads((root/'release-inputs/weathering-0.1.2.json').read_text())
work=root/'.release-build';work.mkdir(exist_ok=True)
def fetch(url,path,sha):
    with urllib.request.urlopen(url,timeout=180) as response,path.open('wb') as out:shutil.copyfileobj(response,out)
    if hashlib.sha256(path.read_bytes()).hexdigest()!=sha:raise RuntimeError('Download checksum mismatch: '+path.name)
source=work/'base-source.zip';fetch(manifest['baseSourceUrl'],source,manifest['baseSourceSha256'])
patch=root/manifest['overlayPath']
if hashlib.sha256(patch.read_bytes()).hexdigest()!=manifest['overlaySha256']:raise RuntimeError('Overlay checksum mismatch')
with zipfile.ZipFile(source) as z:z.extractall(work)
project=work/'weathering'
with zipfile.ZipFile(patch) as z:z.extractall(project)
jdk=work/'jdk.tar.gz';fetch('https://cdn.azul.com/zulu/bin/zulu25.32.17-ca-jdk25.0.2-linux_x64.tar.gz',jdk,'f1752d0051b6ca233625ddb2c18c9170edbe55c5ee6515bfefd8ea0197ee1c20')
with tarfile.open(jdk) as t:t.extractall(work,filter='data')
java=work/'zulu25.32.17-ca-jdk25.0.2-linux_x64';deps=work/'deps'
def run(*args):subprocess.run([str(x) for x in args],check=True)
run(sys.executable,project/'build-direct.py','--java-home',java,'--deps',deps)
version=json.loads((deps/'version.json').read_text());client=version['downloads']['client']
with urllib.request.urlopen(client['url'],timeout=180) as response,(deps/'client.jar').open('wb') as out:shutil.copyfileobj(response,out)
if hashlib.sha1((deps/'client.jar').read_bytes()).hexdigest()!=client['sha1']:raise RuntimeError('Minecraft client checksum mismatch')
run(sys.executable,project/'generate-moss-stages.py','--client-jar',deps/'client.jar')
run(sys.executable,project/'run-integration.py','--java-home',java,'--deps',deps,'--accept-eula')
jar=project/'build/libs/mubbles-weathering-0.1.2.jar'
if hashlib.sha256(jar.read_bytes()).hexdigest()!=manifest['expectedJarSha256']:raise RuntimeError('Build differs from the locally tested release; publication stopped')
out=work/'release';out.mkdir(exist_ok=True);shutil.copy2(jar,out/jar.name)
for name in ['server','restart']:shutil.copy2(project/(name+'-validation.log'),project/'validation'/(name+'-0.1.2.txt'))
archive=out/'mubbles-weathering-0.1.2-source.zip'
with zipfile.ZipFile(archive,'w',zipfile.ZIP_DEFLATED) as z:
    for f in sorted(project.rglob('*')):
        rel=f.relative_to(project)
        if f.is_file() and not any(x in ['build','.gradle','.build-deps','__pycache__'] for x in rel.parts) and f.suffix!='.log':z.write(f,pathlib.Path('weathering')/rel)
(out/'release.json').write_text(json.dumps(manifest['metadata'],indent=2)+'\n')
print('Weathering release reproduced and validated')
