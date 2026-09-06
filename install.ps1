[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [string]$Version = $(if ($env:CX_VERSION) { $env:CX_VERSION } else { 'latest' })
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repository = if ($env:CX_REPOSITORY) { $env:CX_REPOSITORY } else { 'masahide/codex-profile-switcher' }
$apiUrl = if ($env:CX_RELEASE_API_URL) {
    $env:CX_RELEASE_API_URL
} else {
    "https://api.github.com/repos/$repository/releases/latest"
}
$downloadBase = $null
if ($env:CX_RELEASE_DOWNLOAD_BASE_URL) {
    $downloadBase = $env:CX_RELEASE_DOWNLOAD_BASE_URL.TrimEnd('/')
}
$installDir = if ($env:CX_INSTALL_DIR) {
    $env:CX_INSTALL_DIR
} elseif ($env:LOCALAPPDATA) {
    Join-Path $env:LOCALAPPDATA 'cx'
} else {
    Join-Path $HOME '.local\bin'
}

if ($installDir -match '[\r\n;]') {
    throw 'cx installer: CX_INSTALL_DIR contains an invalid character'
}

$headers = @{
    Accept = 'application/vnd.github+json'
    'X-GitHub-Api-Version' = '2022-11-28'
    'User-Agent' = 'cx-installer'
}

if ($Version -eq 'latest') {
    $release = Invoke-RestMethod -Uri $apiUrl -Headers $headers -Method Get
    $Version = [string]$release.tag_name
} elseif (-not $Version.StartsWith('v')) {
    $Version = "v$Version"
}

if ($Version -notmatch '^v[0-9][0-9A-Za-z._-]*$') {
    throw "cx installer: invalid release version: $Version"
}

$processor = if ($env:PROCESSOR_ARCHITEW6432) {
    $env:PROCESSOR_ARCHITEW6432
} else {
    $env:PROCESSOR_ARCHITECTURE
}
$goarch = switch ($processor.ToUpperInvariant()) {
    'AMD64' { 'amd64'; break }
    'ARM64' { 'arm64'; break }
    default { throw "cx installer: unsupported CPU architecture: $processor" }
}

if (-not $downloadBase) {
    $downloadBase = "https://github.com/$repository/releases/download/$Version"
}

$archive = "cx_${Version}_windows_${goarch}.zip"
$temporaryDir = Join-Path ([System.IO.Path]::GetTempPath()) ("cx-install-" + [guid]::NewGuid().ToString('N'))
$archivePath = Join-Path $temporaryDir $archive
$checksumsPath = Join-Path $temporaryDir 'checksums.txt'
$extractDir = Join-Path $temporaryDir 'extract'
$target = Join-Path $installDir 'cx.exe'
$staged = $null

New-Item -ItemType Directory -Path $temporaryDir -Force | Out-Null

try {
    Invoke-WebRequest -UseBasicParsing -Uri "$downloadBase/$archive" -OutFile $archivePath
    Invoke-WebRequest -UseBasicParsing -Uri "$downloadBase/checksums.txt" -OutFile $checksumsPath

    $checksumLine = Get-Content -LiteralPath $checksumsPath | Where-Object {
        $parts = $_ -split '\s+', 2
        $parts.Count -eq 2 -and ($parts[1] -eq $archive -or $parts[1] -eq "*$archive")
    } | Select-Object -First 1
    if (-not $checksumLine) {
        throw "cx installer: checksum not found for $archive"
    }
    $expectedHash = ($checksumLine -split '\s+')[0].ToLowerInvariant()
    if ($expectedHash -notmatch '^[0-9a-f]{64}$') {
        throw "cx installer: invalid checksum for $archive"
    }

    $actualHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $archivePath).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) {
        throw "cx installer: checksum verification failed for $archive"
    }

    New-Item -ItemType Directory -Path $extractDir -Force | Out-Null
    Expand-Archive -LiteralPath $archivePath -DestinationPath $extractDir -Force
    $binary = Join-Path $extractDir 'cx.exe'
    if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) {
        throw 'cx installer: release archive does not contain cx.exe'
    }

    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    $staged = Join-Path $installDir ('.cx.exe.' + [guid]::NewGuid().ToString('N') + '.tmp')
    Copy-Item -LiteralPath $binary -Destination $staged -Force
    Move-Item -LiteralPath $staged -Destination $target -Force
    $staged = $null
} finally {
    if ($staged -and (Test-Path -LiteralPath $staged)) {
        Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path -LiteralPath $temporaryDir) {
        Remove-Item -LiteralPath $temporaryDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

$pathMessage = ''
if ($env:CX_NO_PATH_UPDATE -ne '1') {
    $resolvedInstallDir = (Resolve-Path -LiteralPath $installDir).Path
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $pathEntries = if ($userPath) { @($userPath -split ';' | Where-Object { $_ }) } else { @() }
    $normalizedInstallDir = ($resolvedInstallDir -replace '[\\/]+$', '').ToLowerInvariant()
    $hasPath = $false
    foreach ($entry in $pathEntries) {
        $normalizedEntry = ([Environment]::ExpandEnvironmentVariables($entry) -replace '[\\/]+$', '').ToLowerInvariant()
        if ($normalizedEntry -eq $normalizedInstallDir) {
            $hasPath = $true
            break
        }
    }
    if (-not $hasPath) {
        [Environment]::SetEnvironmentVariable('Path', (@($resolvedInstallDir) + $pathEntries) -join ';', 'User')
        $pathMessage = " Added $resolvedInstallDir to the user PATH."
    }
}

Write-Output "Installed cx $Version for windows/$goarch at $target."
if ($pathMessage) {
    Write-Output $pathMessage
}
Write-Output 'Open a new terminal, then run: cx --help'
