# A loopback-only static server. Requires no software installation or admin rights.
$ErrorActionPreference = 'Stop'
$root = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot 'dist')) + [System.IO.Path]::DirectorySeparatorChar
$listener = New-Object System.Net.Sockets.TcpListener([System.Net.IPAddress]::Loopback, 3000)
try { $listener.Start() } catch { Write-Host 'Port 3000 is already in use. Close any other Workshop window, then try again.'; exit 1 }
Write-Host "Mubble's Workshop is running at http://localhost:3000"
Write-Host 'Leave this window open. Close it to stop the preview.'
Start-Process 'http://localhost:3000'
try {
  while ($true) {
    $client = $listener.AcceptTcpClient()
    try {
      $client.ReceiveTimeout = 5000
      $client.SendTimeout = 5000
      $stream = $client.GetStream()
      $reader = New-Object System.IO.StreamReader($stream, [System.Text.Encoding]::ASCII, $false, 1024, $true)
      $first = $reader.ReadLine()
      if (-not $first) { continue }
      $parts = $first.Split(' ')
      while ($true) { $line = $reader.ReadLine(); if ([string]::IsNullOrEmpty($line)) { break } }
      $status = '200 OK'
      $mime = 'text/plain; charset=utf-8'
      $bytes = [System.Text.Encoding]::UTF8.GetBytes('Not found')
      if ($parts[0] -notin @('GET', 'HEAD')) { $status = '405 Method Not Allowed' } else {
        $relative = [System.Uri]::UnescapeDataString(($parts[1].Split('?')[0])).TrimStart('/')
        if (-not $relative) { $relative = 'index.html' }
        $filePath = [System.IO.Path]::GetFullPath((Join-Path $root $relative))
        if (-not $filePath.StartsWith($root, [System.StringComparison]::OrdinalIgnoreCase)) { $status = '403 Forbidden' }
        elseif (-not [System.IO.File]::Exists($filePath)) { $status = '404 Not Found' }
        else {
          $bytes = [System.IO.File]::ReadAllBytes($filePath)
          switch ([System.IO.Path]::GetExtension($filePath)) {
            '.html' { $mime = 'text/html; charset=utf-8' }
            '.css' { $mime = 'text/css; charset=utf-8' }
            '.js' { $mime = 'text/javascript; charset=utf-8' }
            '.mjs' { $mime = 'text/javascript; charset=utf-8' }
            '.json' { $mime = 'application/json; charset=utf-8' }
            '.svg' { $mime = 'image/svg+xml' }
            default { $mime = 'application/octet-stream' }
          }
        }
      }
      $headers = "HTTP/1.1 $status`r`nContent-Type: $mime`r`nContent-Length: $($bytes.Length)`r`nConnection: close`r`nCache-Control: no-store`r`nX-Content-Type-Options: nosniff`r`n`r`n"
      $headBytes = [System.Text.Encoding]::ASCII.GetBytes($headers)
      $stream.Write($headBytes, 0, $headBytes.Length)
      if ($parts[0] -ne 'HEAD') { $stream.Write($bytes, 0, $bytes.Length) }
      $stream.Flush()
    } catch { Write-Verbose $_.Exception.Message } finally { $client.Close() }
  }
} finally { $listener.Stop() }
