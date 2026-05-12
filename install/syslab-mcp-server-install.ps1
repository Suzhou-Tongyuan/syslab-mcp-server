<#
.SYNOPSIS
One-click installation of MWorks Syslab MCP server configuration for Claude and OpenCode
#>

param(
    [Parameter(Position = 0)]
    [string]$exePath,
    [Parameter(Position = 1)]
    [string]$mworksDir
)

# Helper function: Format JSON string with specified indent size
function Format-JsonIndent {
    param(
        [Parameter(Mandatory = $true)]
        [string]$JsonString,
        [int]$IndentSize = 2
    )

    $indentStr = " " * $IndentSize
    $result = New-Object System.Text.StringBuilder
    $depth = 0
    $inString = $false
    $escapeNext = $false

    for ($i = 0; $i -lt $JsonString.Length; $i++) {
        $char = $JsonString[$i]

        if ($escapeNext) {
            [void]$result.Append($char)
            $escapeNext = $false
            continue
        }

        if ($char -eq '\') {
            [void]$result.Append($char)
            $escapeNext = $true
            continue
        }

        if ($char -eq '"' -and -not $escapeNext) {
            $inString = -not $inString
            [void]$result.Append($char)
            continue
        }

        if ($inString) {
            [void]$result.Append($char)
            continue
        }

        if ($char -match '\s') {
            continue
        }

        if ($char -eq '{' -or $char -eq '[') {
            $j = $i + 1
            while ($j -lt $JsonString.Length -and $JsonString[$j] -match '\s') {
                $j++
            }

            if (
                ($char -eq '{' -and $j -lt $JsonString.Length -and $JsonString[$j] -eq '}') -or
                ($char -eq '[' -and $j -lt $JsonString.Length -and $JsonString[$j] -eq ']')
            ) {
                [void]$result.Append($char)
                [void]$result.Append($JsonString[$j])
                $i = $j
                continue
            }
        }

        if ($char -eq '{' -or $char -eq '[') {
            [void]$result.Append($char)
            $depth++
            [void]$result.AppendLine()
            [void]$result.Append($indentStr * $depth)
            continue
        }

        if ($char -eq '}' -or $char -eq ']') {
            $depth--
            [void]$result.AppendLine()
            [void]$result.Append($indentStr * $depth)
            [void]$result.Append($char)
            continue
        }

        if ($char -eq ':') {
            [void]$result.Append(": ")
            continue
        }

        if ($char -eq ',') {
            [void]$result.Append(",")
            [void]$result.AppendLine()
            [void]$result.Append($indentStr * $depth)
            continue
        }

        [void]$result.Append($char)
    }

    return $result.ToString()
}

function Get-SystemArchitecture {
    $rawArchitecture = $env:PROCESSOR_ARCHITECTURE
    if ([string]::IsNullOrWhiteSpace($rawArchitecture)) {
        try {
            $rawArchitecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
        } catch {
            $rawArchitecture = $null
        }
    }

    if ([string]::IsNullOrWhiteSpace($rawArchitecture)) {
        if ([Environment]::Is64BitOperatingSystem) {
            return "X64"
        }

        return "X86"
    }

    switch -Regex ($rawArchitecture.ToUpperInvariant()) {
        '^ARM64$|^AARCH64$' { return "Arm64" }
        '^AMD64$|^X64$' { return "X64" }
        '^X86$|^I386$|^I686$' { return "X86" }
        default { return $rawArchitecture }
    }
}

function Backup-IfExists {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ConfigPath,
        [Parameter(Mandatory = $true)]
        [string]$Label
    )

    if (Test-Path $ConfigPath) {
        $timestamp = Get-Date -Format "yyyyMMddHHmmss"
        $backupPath = "$ConfigPath.bak.$timestamp"
        Copy-Item -Path $ConfigPath -Destination $backupPath -Force
        Write-Host "$Label existing config backed up to: $backupPath" -ForegroundColor Gray
    }
}

function Ensure-JsonConfigFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ConfigPath
    )

    $configDir = Split-Path -Path $ConfigPath -Parent
    if (-not [string]::IsNullOrWhiteSpace($configDir) -and -not (Test-Path $configDir)) {
        New-Item -ItemType Directory -Path $configDir -Force | Out-Null
    }

    if (-not (Test-Path $ConfigPath)) {
        $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
        [System.IO.File]::WriteAllText($ConfigPath, "{}", $utf8NoBom)
    }
}

function Ensure-TomlConfigFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ConfigPath
    )

    $configDir = Split-Path -Path $ConfigPath -Parent
    if (-not [string]::IsNullOrWhiteSpace($configDir) -and -not (Test-Path $configDir)) {
        New-Item -ItemType Directory -Path $configDir -Force | Out-Null
    }

    if (-not (Test-Path $ConfigPath)) {
        $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
        [System.IO.File]::WriteAllText($ConfigPath, "", $utf8NoBom)
    }
}

function Escape-TomlString {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Value
    )

    return ($Value -replace '\\', '\\\\' -replace '"', '\"')
}

function Set-TomlSection {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ConfigPath,
        [Parameter(Mandatory = $true)]
        [string]$SectionName,
        [Parameter(Mandatory = $true)]
        [string[]]$SectionLines
    )

    $existingContent = ""
    if (Test-Path $ConfigPath) {
        $existingContent = Get-Content $ConfigPath -Raw -Encoding UTF8
    }

    $normalizedContent = $existingContent -replace "`r`n", "`n"
    $sectionHeader = "[$SectionName]"
    $newSection = ((($sectionHeader) + "`n" + ($SectionLines -join "`n")).TrimEnd()) + "`n"

    if ([string]::IsNullOrWhiteSpace($normalizedContent)) {
        $updatedContent = $newSection + "`n"
    } else {
        $pattern = "(?ms)^[ \t]*\[$([regex]::Escape($SectionName))\][ \t]*\n.*?(?=^[ \t]*\[|\z)"
        if ([regex]::IsMatch($normalizedContent, $pattern)) {
            $updatedContent = [regex]::Replace($normalizedContent, $pattern, $newSection + "`n", 1)
        } else {
            $trimmedContent = $normalizedContent.TrimEnd("`n")
            $updatedContent = $trimmedContent + "`n`n" + $newSection + "`n"
        }
    }

    $finalContent = $updatedContent -replace "`n", "`r`n"
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($ConfigPath, $finalContent, $utf8NoBom)
}

# Get current script directory
$scriptDir = $PSScriptRoot
$nonInteractive = $PSBoundParameters.ContainsKey("exePath") -or $PSBoundParameters.ContainsKey("mworksDir")

if ([string]::IsNullOrWhiteSpace($exePath)) {
    $osArchitecture = Get-SystemArchitecture

    switch ($osArchitecture) {
        "Arm64" {
            $exeName = "syslab-mcp-server-winarm64.exe"
        }
        "X64" {
            $exeName = "syslab-mcp-server-win64.exe"
        }
        default {
            Write-Error "Error: Unsupported system architecture: $osArchitecture. Only X64 and Arm64 are supported."
            if (-not $nonInteractive) {
                pause
            }
            exit 1
        }
    }

    $defaultExePath = Join-Path $scriptDir $exeName
    if (Test-Path $defaultExePath) {
        $exePath = $defaultExePath
    } else {
        $exePath = Read-Host "Enter Syslab MCP executable path (default: $defaultExePath)"
        if ([string]::IsNullOrWhiteSpace($exePath)) {
            $exePath = $defaultExePath
        }
    }
} else {
    $exePath = [System.IO.Path]::GetFullPath($exePath)
    $osArchitecture = "Custom"
}

if (-not (Test-Path $exePath)) {
    Write-Error "Error: executable not found: $exePath"
    if (-not $nonInteractive) {
        pause
    }
    exit 1
}

Write-Host "=== MWORKS Syslab MCP One-Click Configuration Tool ===" -ForegroundColor Cyan
Write-Host "Detected Architecture: $osArchitecture" -ForegroundColor Gray
Write-Host "EXE Path: $exePath`n" -ForegroundColor Gray

# Ask user for Syslab install path
$defaultMworksDir = Split-Path -Path (Split-Path -Path (Split-Path -Path $exePath -Parent) -Parent) -Parent
if ([string]::IsNullOrWhiteSpace($mworksDir)) {
    if ($env:SYSLAB_HOME) {
        $defaultMworksDir = $env:SYSLAB_HOME
    }

    if ($nonInteractive) {
        $mworksDir = $defaultMworksDir
    } else {
        $mworksDir = Read-Host "Enter Syslab install path (default: $defaultMworksDir)"
        if ([string]::IsNullOrWhiteSpace($mworksDir)) {
            $mworksDir = $defaultMworksDir
        }
    }
} else {
    $mworksDir = [System.IO.Path]::GetFullPath($mworksDir)
}

# Verify path exists
if (-not (Test-Path $mworksDir)) {
    Write-Warning "Warning: The specified Syslab install path $mworksDir does not exist, please confirm if it is correct"
    if (-not $nonInteractive) {
        $continue = Read-Host "Continue? (Y/N, default Y)"
        if ($continue -eq "N" -or $continue -eq "n") {
            exit 0
        }
    }
}

Write-Host "`nUsing configuration:" -ForegroundColor Yellow
Write-Host "Syslab Install Path: $mworksDir"
Write-Host ""

$appliedConfig = $false
$appliedClients = @()

# 1. Configure Claude (.claude.json)
$claudeConfigPath = Join-Path $env:USERPROFILE ".claude.json"
if ($nonInteractive -or (Test-Path $claudeConfigPath)) {
    $configureClaude = $true
    Write-Host "1. Configuring Claude..." -ForegroundColor Yellow
    if (Test-Path $claudeConfigPath) {
        Write-Host "Detected existing Claude config: $claudeConfigPath" -ForegroundColor Cyan
    } else {
        Write-Host "Claude config will be created: $claudeConfigPath" -ForegroundColor Cyan
    }

    if (-not $nonInteractive) {
        $confirmClaude = Read-Host "Configure Claude? (Y/N, default Y)"
        if ($confirmClaude -eq "N" -or $confirmClaude -eq "n") {
            $configureClaude = $false
            Write-Host "Skipping Claude configuration." -ForegroundColor Gray
        }
    }

    if ($configureClaude) {
        Ensure-JsonConfigFile -ConfigPath $claudeConfigPath
        $claudeConfig = Get-Content $claudeConfigPath -Raw -Encoding UTF8 | ConvertFrom-Json
    }
} else {
    $configureClaude = $false
}

if ($configureClaude) {
    if (-not $nonInteractive) {
        Backup-IfExists -ConfigPath $claudeConfigPath -Label "Claude"
    }

    if (-not $claudeConfig.mcpServers) {
        $claudeConfig | Add-Member -MemberType NoteProperty -Name "mcpServers" -Value ([PSCustomObject]@{}) -Force
    }

    $claudeConfig.mcpServers | Add-Member -MemberType NoteProperty -Name "syslab" -Value ([PSCustomObject]@{
        type = "stdio"
        command = $exePath
        args = @(
            "--syslab-root", $mworksDir
        )
    }) -Force

    $jsonContent = $claudeConfig | ConvertTo-Json -Depth 10
    $jsonContent = Format-JsonIndent -JsonString $jsonContent -IndentSize 2
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($claudeConfigPath, $jsonContent, $utf8NoBom)
    $appliedConfig = $true
    $appliedClients += "Claude Code"
    Write-Host "Claude configuration updated: $claudeConfigPath" -ForegroundColor Green
}

# 2. Configure OpenCode (opencode.json)
$opencodeConfigDir = Join-Path $env:USERPROFILE ".config\opencode"
$opencodeConfigPath = Join-Path $opencodeConfigDir "opencode.json"

if ($nonInteractive -or (Test-Path $opencodeConfigPath)) {
    $configureOpenCode = $true
    Write-Host "`n2. Configuring OpenCode..." -ForegroundColor Yellow
    if (Test-Path $opencodeConfigPath) {
        Write-Host "Detected existing OpenCode config: $opencodeConfigPath" -ForegroundColor Cyan
    } else {
        Write-Host "OpenCode config will be created: $opencodeConfigPath" -ForegroundColor Cyan
    }

    if (-not $nonInteractive) {
        $confirmOpenCode = Read-Host "Configure OpenCode? (Y/N, default Y)"
        if ($confirmOpenCode -eq "N" -or $confirmOpenCode -eq "n") {
            $configureOpenCode = $false
            Write-Host "Skipping OpenCode configuration." -ForegroundColor Gray
        }
    }

    if ($configureOpenCode) {
        Ensure-JsonConfigFile -ConfigPath $opencodeConfigPath
        $opencodeConfig = Get-Content $opencodeConfigPath -Raw -Encoding UTF8 | ConvertFrom-Json
    }
} else {
    $configureOpenCode = $false
}

if ($configureOpenCode) {
    if (-not $nonInteractive) {
        Backup-IfExists -ConfigPath $opencodeConfigPath -Label "OpenCode"
    }

    if (-not $opencodeConfig.mcp) {
        $opencodeConfig | Add-Member -MemberType NoteProperty -Name "mcp" -Value ([PSCustomObject]@{}) -Force
    }

    $opencodeConfig.mcp | Add-Member -MemberType NoteProperty -Name "syslab" -Value ([PSCustomObject]@{
        type = "local"
        command = @(
            $exePath,
            "--syslab-root", $mworksDir
        )
    }) -Force

    $jsonContent = $opencodeConfig | ConvertTo-Json -Depth 10
    $jsonContent = Format-JsonIndent -JsonString $jsonContent -IndentSize 2
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($opencodeConfigPath, $jsonContent, $utf8NoBom)
    $appliedConfig = $true
    $appliedClients += "OpenCode"
    Write-Host "OpenCode configuration updated: $opencodeConfigPath" -ForegroundColor Green
}

# 3. Configure Codex (config.toml)
$codexConfigPath = Join-Path $env:USERPROFILE ".codex\config.toml"
if ($nonInteractive -or (Test-Path $codexConfigPath)) {
    $configureCodex = $true
    Write-Host "`n3. Configuring Codex..." -ForegroundColor Yellow
    if (Test-Path $codexConfigPath) {
        Write-Host "Detected existing Codex config: $codexConfigPath" -ForegroundColor Cyan
    } else {
        Write-Host "Codex config will be created: $codexConfigPath" -ForegroundColor Cyan
    }

    if (-not $nonInteractive) {
        $confirmCodex = Read-Host "Configure Codex? (Y/N, default Y)"
        if ($confirmCodex -eq "N" -or $confirmCodex -eq "n") {
            $configureCodex = $false
            Write-Host "Skipping Codex configuration." -ForegroundColor Gray
        }
    }
} else {
    $configureCodex = $false
}

if ($configureCodex) {
    Ensure-TomlConfigFile -ConfigPath $codexConfigPath
    if (-not $nonInteractive) {
        Backup-IfExists -ConfigPath $codexConfigPath -Label "Codex"
    }

    $escapedExePath = Escape-TomlString -Value $exePath
    $escapedMworksDir = Escape-TomlString -Value $mworksDir
    Set-TomlSection -ConfigPath $codexConfigPath -SectionName "mcp_servers.syslab" -SectionLines @(
        "command = ""$escapedExePath"""
        "args = [""--syslab-root"", ""$escapedMworksDir""]"
    )

    $appliedConfig = $true
    $appliedClients += "Codex"
    Write-Host "Codex configuration updated: $codexConfigPath" -ForegroundColor Green
}

if (-not (Test-Path $claudeConfigPath) -and -not (Test-Path $opencodeConfigPath) -and -not (Test-Path $codexConfigPath)) {
    Write-Host "No existing Claude, OpenCode, or Codex configuration files were detected. Skipped all configuration steps." -ForegroundColor Yellow
}

Write-Host "`nConfiguration completed!" -ForegroundColor Cyan
if ($appliedConfig) {
    $clientList = $appliedClients -join " and "
    Write-Host "Please restart $clientList to load the new MCP configuration." -ForegroundColor Gray
}
Write-Host "To uninstall, simply delete the syslab field in the corresponding configuration file." -ForegroundColor Gray
if (-not $nonInteractive) {
    pause
}
