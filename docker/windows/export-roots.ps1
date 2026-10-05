# Runs in a servercore build stage. Nanoserver has no certutil or PowerShell, so the roots are
# added to the LocalMachine Root store here and exported as .reg files for `reg import` in the
# final image. Without GlobalSign Root CA in the store, crypt32 fetches it over plain HTTP
# (AIA), which bypasses HTTPS_PROXY and hangs behind proxy-only egress (CI-24827).
$ErrorActionPreference = 'Stop'

$expected = @{
  'B1BC968BD4F49D622AA89A81F2150152A41D829C' = 'GlobalSign Root CA'
  'E58C1CC4913B38634BE9106EE3AD8E6B9DD9814A' = 'GTS Root R1'
  '77D30367B5E00C15F60C3861DF7CE13B92464D47' = 'GTS Root R4'
}

$outDir = 'C:\roots'
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

$exported = @{}
Get-ChildItem (Join-Path $PSScriptRoot 'certs\*.crt') | ForEach-Object {
  $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2 $_.FullName
  if (-not $expected.ContainsKey($cert.Thumbprint)) {
    throw "Unexpected certificate $($_.Name) with thumbprint $($cert.Thumbprint)"
  }

  certutil -addstore -f Root $_.FullName | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "certutil -addstore failed for $($_.Name)" }

  $key = "HKLM\SOFTWARE\Microsoft\SystemCertificates\ROOT\Certificates\$($cert.Thumbprint)"
  reg export $key (Join-Path $outDir "$($cert.Thumbprint).reg") /y | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "reg export failed for $($_.Name)" }

  Write-Host "Exported $($expected[$cert.Thumbprint]) ($($cert.Thumbprint))"
  $exported[$cert.Thumbprint] = $true
}

$missing = $expected.Keys | Where-Object { -not $exported.ContainsKey($_) }
if ($missing) { throw "Missing root certificates: $($missing -join ', ')" }
