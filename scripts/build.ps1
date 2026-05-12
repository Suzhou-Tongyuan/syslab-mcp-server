$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $PSScriptRoot
$OutDir = Join-Path $Root "bin"
$Targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64"; OutFile = "syslab-mcp-server-win64.exe" },
    @{ GOOS = "windows"; GOARCH = "arm64"; OutFile = "syslab-mcp-server-winarm64.exe" },
    @{ GOOS = "linux"; GOARCH = "amd64"; OutFile = "syslab-mcp-server-glnxa64" },
    @{ GOOS = "linux"; GOARCH = "arm64"; OutFile = "syslab-mcp-server-glnxarm64" }
)
$GoCache = Join-Path $Root ".cache\go-build"
$VersionFile = Join-Path $Root "VERSION"

function Resolve-Version {
    if ($env:VERSION) {
        return $env:VERSION.Trim()
    }

    if ($env:CI_COMMIT_TAG) {
        return $env:CI_COMMIT_TAG.Trim()
    }

    if (Test-Path $VersionFile) {
        return (Get-Content -Raw $VersionFile).Trim()
    }

    return "0.1.0"
}

function Resolve-GoExe {
    $candidates = @(
        (Join-Path $Root "tools\go\bin\go.exe")
    )

    foreach ($candidate in $candidates) {
        if ($candidate -and (Test-Path $candidate)) {
            return $candidate
        }
    }

    $cmd = Get-Command go.exe -ErrorAction SilentlyContinue
    if ($cmd) {
        return $cmd.Source
    }

    throw "Go toolchain not found. Checked repository tools and PATH."
}

$GoExe = Resolve-GoExe
$Version = Resolve-Version

New-Item -ItemType Directory -Force $OutDir | Out-Null
New-Item -ItemType Directory -Force $GoCache | Out-Null
$env:GOCACHE = $GoCache
$env:CGO_ENABLED = if ($env:CGO_ENABLED) { $env:CGO_ENABLED } else { "0" }

function Build-Target($Target) {
    $outPath = Join-Path $OutDir $Target.OutFile
    $oldGOOS = $env:GOOS
    $oldGOARCH = $env:GOARCH

    try {
        $env:GOOS = $Target.GOOS
        $env:GOARCH = $Target.GOARCH
        & $GoExe build -trimpath -ldflags "-s -w -X main.version=$Version" -o $outPath (Join-Path $Root "cmd\syslab-mcp-core-server")
        Write-Host "Built $outPath"
    }
    finally {
        $env:GOOS = $oldGOOS
        $env:GOARCH = $oldGOARCH
    }
}

foreach ($target in $Targets) {
    Build-Target $target
}
