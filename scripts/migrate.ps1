$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot

$container = "esppd-postgres"
$dbUser = if ($env:POSTGRES_USER) { $env:POSTGRES_USER } else { "esppd" }
$dbName = if ($env:POSTGRES_DB) { $env:POSTGRES_DB } else { "esppd" }

$migrationsPath = Join-Path $root "migrations"
$files = Get-ChildItem -Path $migrationsPath -Filter "*.sql" | Sort-Object Name

foreach ($f in $files) {
  Write-Host "Applying $($f.Name)"
  docker exec -i $container psql -U $dbUser -d $dbName < $f.FullName
}
