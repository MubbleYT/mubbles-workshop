"""Reproduce and test the authorized Borderless release from a pinned source input."""
import hashlib,json,pathlib,shutil,subprocess,sys,tarfile,urllib.request,zipfile
root=pathlib.Path(__file__).resolve().parent.parent
manifest=json.loads((root/'release-inputs/borderless-0.1.0.json').read_text())
work=root/'.borderless-release-build';work.mkdir(exist_ok=True)
source=root/manifest['sourcePath']
if hashlib.sha256(source.read_bytes()).hexdigest()!=manifest['sourceSha256']:raise RuntimeError('Source checksum mismatch')
with zipfile.ZipFile(source) as z:z.extractall(work)
project=work/'borderless'
jdk=work/'jdk.tar.gz'
with urllib.request.urlopen('https://cdn.azul.com/zulu/bin/zulu25.32.17-ca-jdk25.0.2-linux_x64.tar.gz',timeout=180) as response,jdk.open('wb') as out:shutil.copyfileobj(response,out)
if hashlib.sha256(jdk.read_bytes()).hexdigest()!='f1752d0051b6ca233625ddb2c18c9170edbe55c5ee6515bfefd8ea0197ee1c20':raise RuntimeError('JDK checksum mismatch')
with tarfile.open(jdk) as t:t.extractall(work,filter='data')
java=work/'zulu25.32.17-ca-jdk25.0.2-linux_x64';deps=work/'deps'
subprocess.run([sys.executable,str(project/'run-integration.py'),'--java-home',str(java),'--deps',str(deps),'--accept-eula'],check=True)
jar=project/'build/libs/mubbles-borderless-0.1.0.jar'
if hashlib.sha256(jar.read_bytes()).hexdigest()!=manifest['expectedJarSha256']:raise RuntimeError('Build differs from locally tested release')
out=work/'release';out.mkdir(exist_ok=True);shutil.copy2(jar,out/jar.name)
shutil.copy2(source,out/'mubbles-borderless-0.1.0-source.zip')
(out/'release.json').write_text(json.dumps(manifest['metadata'],indent=2)+'\n')
print('Borderless release reproduced and validated')
