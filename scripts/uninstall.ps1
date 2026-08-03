$ErrorActionPreference = "Stop"

$BinaryName = "argus.exe"
$InstallDir = Join-Path $HOME ".local\bin"
$BinaryPath = Join-Path $InstallDir $BinaryName

if (Test-Path $BinaryPath) {
    Remove-Item -Force $BinaryPath
    Write-Host "removed $BinaryName from $BinaryPath"
}
else {
    Write-Host "$BinaryName was not found at $BinaryPath"
}
