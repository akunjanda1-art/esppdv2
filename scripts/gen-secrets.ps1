$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

# 1) Ensure .env exists
if (-not (Test-Path .\.env)) {
  Copy-Item -Force .\.env.example .\.env
  Write-Host "Created .env from .env.example"
}

# 2) Ensure AES-256-GCM key (32 bytes) exists for field-level encryption
$envLines = Get-Content .\.env
$currentEncKey = $null
foreach ($line in $envLines) {
  if ($line -match '^DATA_ENC_KEY_BASE64=') {
    $currentEncKey = $line.Substring('DATA_ENC_KEY_BASE64='.Length)
    break
  }
}

$needsEncKey = (-not $currentEncKey) -or ($currentEncKey -eq '') -or ($currentEncKey -like 'REPLACE_*')
if ($needsEncKey) {
  $key = New-Object byte[] 32
  $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
  $rng.GetBytes($key)
  $rng.Dispose()
  $currentEncKey = [Convert]::ToBase64String($key)
}

$hasEncKeyLine = $false
$envLines = $envLines | ForEach-Object {
  if ($_ -match '^DATA_ENC_KEY_BASE64=') {
    $hasEncKeyLine = $true
    "DATA_ENC_KEY_BASE64=$currentEncKey"
  } else {
    $_
  }
}
if (-not $hasEncKeyLine) {
  $envLines += "DATA_ENC_KEY_BASE64=$currentEncKey"
}

# Ensure JWT private key passphrase exists for dev (ssh-keygen PEM export needs a non-empty passphrase here).
$jwtPass = $null
$hasJwtPass = $false
$envLines = $envLines | ForEach-Object {
  if ($_ -match '^JWT_PRIVATE_KEY_PASSPHRASE=') {
    $hasJwtPass = $true
    $val = $_.Substring('JWT_PRIVATE_KEY_PASSPHRASE='.Length)
    if ($val) {
      $jwtPass = $val
      $_
    } else {
      $jwtPass = "devkey"
      "JWT_PRIVATE_KEY_PASSPHRASE=$jwtPass"
    }
  } else {
    $_
  }
}
if (-not $hasJwtPass) {
  $jwtPass = "devkey"
  $envLines += "JWT_PRIVATE_KEY_PASSPHRASE=$jwtPass"
}
if (-not $jwtPass) {
  $jwtPass = "devkey"
}

$envLines | Set-Content -Encoding UTF8 -Path .\.env
if ($needsEncKey) {
  Write-Host "Generated DATA_ENC_KEY_BASE64 in .env"
} else {
  Write-Host "DATA_ENC_KEY_BASE64 already set in .env"
}

# 3) Generate JWT RSA keys for dev
$secretsDir = Join-Path $root 'secrets'
New-Item -ItemType Directory -Force -Path $secretsDir | Out-Null

$privPath = Join-Path $secretsDir 'jwt_private.pem'
$pubPath  = Join-Path $secretsDir 'jwt_public.pem'

if ((-not (Test-Path $privPath)) -or (-not (Test-Path $pubPath))) {
  # Use ssh-keygen (ships with Windows OpenSSH) to avoid .NET Framework export limitations.
  ssh-keygen -t rsa -b 2048 -m PEM -f $privPath -N $jwtPass -q | Out-Null
  ssh-keygen -e -m PKCS8 -f "$privPath.pub" | Set-Content -Encoding ASCII -Path $pubPath
  Remove-Item -Force "$privPath.pub"
  Write-Host "Generated dev JWT keys under secrets/ (jwt_private.pem, jwt_public.pem)"
} else {
  Write-Host "JWT keys already exist under secrets/"
}

Write-Host "Done."
