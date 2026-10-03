import http from 'node:http';
import {readFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
const root=fileURLToPath(new URL('./dist/',import.meta.url));
const types={'.html':'text/html; charset=utf-8','.js':'text/javascript; charset=utf-8','.mjs':'text/javascript; charset=utf-8','.json':'application/json; charset=utf-8','.css':'text/css; charset=utf-8','.svg':'image/svg+xml','.png':'image/png'};
const server=http.createServer(async(req,res)=>{try{if(!['GET','HEAD'].includes(req.method)){res.writeHead(405);res.end('Method not allowed');return}const pathname=decodeURIComponent(new URL(req.url,'http://localhost').pathname);const file=path.resolve(root,'.'+(pathname==='/'?'/index.html':pathname));if(!file.startsWith(root)){res.writeHead(403);res.end('Forbidden');return}const content=await readFile(file);res.writeHead(200,{'Content-Type':types[path.extname(file)]||'application/octet-stream','Content-Length':content.length,'X-Content-Type-Options':'nosniff','Cache-Control':'no-store'});res.end(req.method==='HEAD'?undefined:content)}catch{res.writeHead(404);res.end('Not found')}});
server.on('error',error=>{console.error(error.code==='EADDRINUSE'?'Port 3000 is already in use. Close the other preview and try again.':error.message);process.exit(1)});
server.listen(3000,'127.0.0.1',()=>console.log("Mubble's Workshop is running at http://localhost:3000\nLeave this window open. Press Ctrl+C to stop."));
