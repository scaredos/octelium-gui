# Run elevated by the installer. Installs the Octelium CLI into an admin-only
# folder and registers `octelium daemon` as a service, so connecting from the
# GUI never needs admin rights. The service must not run a binary from a
# user-writable folder such as the default %USERPROFILE%\.local\bin.
param([Parameter(Mandatory)][string]$AppDir)
$ErrorActionPreference = 'Stop'
$Bin = Join-Path $AppDir 'bin'
$Svc = 'OcteliumDaemon'

if (Get-Service $Svc -ErrorAction SilentlyContinue) { Stop-Service $Svc -Force }

# Same script as `iwr https://octelium.com/install.ps1 -useb | iex`, pointed at Program Files.
$script = (Invoke-WebRequest https://octelium.com/install.ps1 -UseBasicParsing).Content
& ([scriptblock]::Create($script)) -InstallDir $Bin

if (-not (Get-Service $Svc -ErrorAction SilentlyContinue)) {
    New-Service -Name $Svc -DisplayName 'Octelium daemon' -StartupType Automatic `
        -BinaryPathName "`"$Bin\octelium.exe`" daemon" | Out-Null
}
Start-Service $Svc
