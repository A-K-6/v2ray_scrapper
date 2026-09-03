# v2rays Windows installer:
#   irm https://raw.githubusercontent.com/A-K-6/v2ray_scrapper/main/install.ps1 | iex
param(
  [string]$Repo = $(if ($env:V2RAYS_REPO) { $env:V2RAYS_REPO } else { "A-K-6/v2ray_scrapper" }),
  [string]$Version = $(if ($env:V2RAYS_VERSION) { $env:V2RAYS_VERSION } else { "latest" }),
  [string]$InstallDir = $(if ($env:V2RAYS_INSTALL_DIR) { $env:V2RAYS_INSTALL_DIR } else { "$env:LocalAppData\v2rays\bin" })
)

$ErrorActionPreference = "Stop"

function Resolve-Version($repo, $ver) {
  if ($ver -ne "latest") { return $ver }
  $api = "https://api.github.com/repos/$repo/releases/latest"
  try {
    $rel = Invoke-RestMethod -Uri $api -UseBasicParsing
    return $rel.tag_name
  } catch {
    throw "Could not resolve latest release. Set V2RAYS_VERSION=vX.Y.Z explicitly. $_"
  }
}

$arch = "amd64"
if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -match "Arm64") { $arch = "arm64" }

$tag = Resolve-Version $Repo $Version
$asset = "v2rays-$($tag.TrimStart('v'))-windows-$arch.zip"
$url = "https://github.com/$Repo/releases/download/$tag/$asset"

Write-Host "Downloading $url"
$tmp = Join-Path $env:TEMP ("v2rays-" + [Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
  $zip = Join-Path $tmp $asset
  Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing
  Expand-Archive -Path $zip -DestinationPath $tmp -Force
  New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
  Copy-Item (Join-Path $tmp "v2rays.exe") (Join-Path $InstallDir "v2rays.exe") -Force
  Write-Host "Installed to $InstallDir\v2rays.exe"

  $path = [Environment]::GetEnvironmentVariable("Path", "User")
  if ($path -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$path;$InstallDir", "User")
    $env:Path += ";$InstallDir"
    Write-Host "Added $InstallDir to user PATH (restart terminal to take effect)"
  }
  & (Join-Path $InstallDir "v2rays.exe") version
  Write-Host "Next: v2rays config init; v2rays doctor; v2rays tui"
} finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
