param(
    [string]$Version = "v-test",
    [ValidateSet("x64", "ia32", "arm64")]
    [string]$Arch = "x64",
    [switch]$SkipNpmInstall,
    [switch]$SkipGoTidy,
    [switch]$SkipMake
)

$ErrorActionPreference = "Stop"

function Step([string]$msg) {
    Write-Host "`n==> $msg" -ForegroundColor Cyan
}

function Ensure-Cmd([string]$name) {
    if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
        throw "Required command '$name' is not available in PATH."
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

Step "Checking required tools"
Ensure-Cmd "node"
Ensure-Cmd "npm"
Ensure-Cmd "go"

Step "Printing tool versions"
node -v
npm -v
go version

if (-not $SkipNpmInstall) {
    Step "Installing Node dependencies"
    npm install
}

Step "Checking for unresolved merge conflict markers"
$conflicts = Select-String -Path (Join-Path $repoRoot "src-go\**\*.go") -Pattern '^(<<<<<<<|=======|>>>>>>>)' -SimpleMatch:$false -ErrorAction SilentlyContinue
if ($conflicts) {
    Write-Host "Found unresolved merge conflict markers in Go sources:" -ForegroundColor Red
    $conflicts | ForEach-Object { Write-Host ("  {0}:{1}: {2}" -f $_.Path, $_.LineNumber, $_.Line.Trim()) -ForegroundColor Red }
    throw "Resolve merge conflict markers before building."
}

Step "Ensuring Electron Forge Squirrel maker dependency"
$pkg = Get-Content -Raw -Path "package.json" | ConvertFrom-Json
$hasMaker = $false
if ($pkg.devDependencies -and $pkg.devDependencies.PSObject.Properties.Name -contains "@electron-forge/maker-squirrel") {
    $hasMaker = $true
}

if (-not $hasMaker) {
    Write-Host "@electron-forge/maker-squirrel is missing from devDependencies; installing..." -ForegroundColor Yellow
    npm i -D @electron-forge/maker-squirrel@^7.8.1
}

Set-Location (Join-Path $repoRoot "src-go")

if (-not $SkipGoTidy) {
    Step "Running go mod tidy"
    go mod tidy
}

Step "Building backend px.exe"
$ldflags = "-X github.com/legiz-ru/prizrak-box/api.Version=$Version"
$env:CGO_ENABLED = "0"
go build -tags=with_gvisor -trimpath -ldflags $ldflags -o px.exe

if (-not (Test-Path "px.exe")) {
    throw "Backend build failed: src-go/px.exe not found."
}

Set-Location $repoRoot

if (-not $SkipMake) {
    Step "Building Windows installer via Electron Forge"
    npm run make -- --platform=win32 --arch=$Arch
}

Step "Searching generated artifacts"
$artifacts = Get-ChildItem -Path (Join-Path $repoRoot "out") -Recurse -File -ErrorAction SilentlyContinue |
    Where-Object { $_.Extension -in @(".exe", ".msi", ".nupkg", ".zip") } |
    Select-Object FullName

if ($artifacts) {
    $artifacts | Format-Table -AutoSize
} else {
    Write-Host "No artifacts found in ./out" -ForegroundColor Yellow
}

Write-Host "`nDone." -ForegroundColor Green
