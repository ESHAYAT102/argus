$ErrorActionPreference = "Stop"

$RepoUrl = "https://github.com/ESHAYAT102/argus.git"
$BinaryName = "argus.exe"
$InstallDir = Join-Path $HOME ".local\bin"
$CloneDir = $null

function Test-Command {
    param([string]$Name)

    [bool](Get-Command $Name -ErrorAction SilentlyContinue)
}

function Confirm-Install {
    param([string]$Message)
    while ($true) {
        $Answer = Read-Host "$Message [Y/n]"
        switch -Regex ($Answer) {
            "^\s*$" { return $true }
            "^[Yy]([Ee][Ss])?$" { return $true }
            "^[Nn]([Oo])?$" { return $false }
            default { Write-Host "please answer Y or n" }
        }
    }
}

function Update-PathFromEnvironment {
    $MachinePath = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $Paths = @($MachinePath, $UserPath) | Where-Object { $_ }
    if ($Paths.Count -gt 0) {
        $env:Path = $Paths -join ";"
    }
}

function Install-DevDependencies {
    if (Test-Command winget) {
        winget install --id GoLang.Go --exact --accept-package-agreements --accept-source-agreements
        winget install --id Git.Git --exact --accept-package-agreements --accept-source-agreements
        Update-PathFromEnvironment
        return
    }

    if (Test-Command choco) {
        choco install golang git -y
        Update-PathFromEnvironment
        return
    }

    if (Test-Command scoop) {
        scoop install go git
        Update-PathFromEnvironment
        return
    }

    throw "Go and git are required, but no supported package manager was found. Install winget, Chocolatey, or Scoop, then rerun this script."
}

function Ensure-DevDependencies {
    if ((Test-Command go) -and (Test-Command git)) {
        return
    }

    $Missing = @()
    if (-not (Test-Command go)) {
        $Missing += "go"
    }
    if (-not (Test-Command git)) {
        $Missing += "git"
    }
    $MissingText = $Missing -join " and "

    if (-not (Confirm-Install "Install missing required software ($MissingText) now?")) {
        throw "$MissingText is required to install argus"
    }

    Install-DevDependencies

    if (-not (Test-Command go)) {
        throw "go is still unavailable after installation"
    }
    if (-not (Test-Command git)) {
        throw "git is still unavailable after installation"
    }
}

try {
    Ensure-DevDependencies

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

    $CloneDir = Join-Path ([System.IO.Path]::GetTempPath()) "argus-$([System.Guid]::NewGuid())"
    Write-Host "cloning $RepoUrl"
    git clone --depth 1 $RepoUrl $CloneDir
    Write-Host "building argus"
    Push-Location $CloneDir
    try {
        go build -o (Join-Path $InstallDir $BinaryName) ./cmd/argus
    }
    finally {
        Pop-Location
    }

    Write-Host "installed $BinaryName to $(Join-Path $InstallDir $BinaryName)"

    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $PathEntries = @()
    if ($UserPath) {
        $PathEntries = $UserPath -split ";"
    }
    if ($PathEntries -notcontains $InstallDir) {
        $NewPath = if ($UserPath) { "$UserPath;$InstallDir" } else { $InstallDir }
        [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
        $env:Path = "$env:Path;$InstallDir"
        Write-Host "added $InstallDir to your user PATH"
        Write-Host "restart your terminal before running argus"
    }
}
finally {
    if ($CloneDir -and (Test-Path $CloneDir)) {
        Remove-Item -Recurse -Force $CloneDir
    }
}
