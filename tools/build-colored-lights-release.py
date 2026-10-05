"""Reconstruct the exact locally validated V0.0.9 shader ZIP from pinned inputs."""
from pathlib import Path
import base64,hashlib,json,shutil,urllib.request,zipfile
root=Path(__file__).resolve().parent.parent
work=root/'.release-build/colored-lights';work.mkdir(parents=True,exist_ok=True)
overlay=root/'release-inputs/colored-lights-0.0.9-overlay.zip'
with zipfile.ZipFile(overlay) as patch:
    assert patch.testzip() is None
    manifest=json.loads(patch.read('release-manifest.json'))
    base=work/'base.zip'
    with urllib.request.urlopen(manifest['baseUrl'],timeout=180) as source,base.open('wb') as out:
        shutil.copyfileobj(source,out)
    assert hashlib.sha256(base.read_bytes()).hexdigest()==manifest['baseSha256'],'Base checksum mismatch'
    temporary=work/'release.tmp'
    with zipfile.ZipFile(base) as original,zipfile.ZipFile(temporary,'w',zipfile.ZIP_DEFLATED,compresslevel=6) as archive:
        for entry in manifest['entries']:
            info=zipfile.ZipInfo(entry['filename'],tuple(entry['date_time']))
            for key,value in entry.items():
                if key not in ['filename','date_time','overlay']:
                    setattr(info,key,base64.b64decode(value) if key in ['extra','comment'] else value)
            data=(patch if entry['overlay'] else original).read(info.filename)
            archive.writestr(info,data,compresslevel=6)
    assert hashlib.sha256(temporary.read_bytes()).hexdigest()==manifest['targetSha256'],'Release checksum mismatch'
    with zipfile.ZipFile(temporary) as archive:
        assert archive.testzip() is None
        assert 'shaders/shaders.properties' in archive.namelist()
        assert json.loads(archive.read('validation/compile-results.json'))['passed']==135
        assert json.loads(archive.read('validation/audit-results.json'))['locked_end_library_identical_to_base']
    temporary.replace(work/manifest['targetName'])
print('Reconstructed exact validated V0.0.9 ZIP; checksum and archive passed.')
