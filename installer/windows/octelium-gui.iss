; Build: go build -ldflags "-H windowsgui" -o octelium-gui.exe .  then  iscc installer\windows\octelium-gui.iss
#define AppVersion GetEnv("VERSION") != "" ? GetEnv("VERSION") : "0.1.0"

[Setup]
AppId={{4D7010A9-ECC4-48D0-8A8D-BD1A104B617D}
AppName=Octelium GUI
AppVersion={#AppVersion}
AppPublisher=Octelium GUI contributors
DefaultDirName={autopf}\Octelium GUI
DisableProgramGroupPage=yes
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64compatible
ChangesEnvironment=yes
OutputDir=..\..\dist
OutputBaseFilename=octelium-gui-setup-{#AppVersion}
LicenseFile=..\..\LICENSE
WizardStyle=modern

[Files]
Source: "..\..\octelium-gui.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "setup.ps1"; DestDir: "{app}"; Flags: ignoreversion

[Tasks]
Name: "autostart"; Description: "Start Octelium in the system tray when Windows starts"

[Icons]
Name: "{autoprograms}\Octelium"; Filename: "{app}\octelium-gui.exe"
Name: "{commonstartup}\Octelium"; Filename: "{app}\octelium-gui.exe"; Parameters: "--tray"; Tasks: autostart

[Registry]
; Put octelium.exe on every user's PATH.
Root: HKLM; Subkey: "SYSTEM\CurrentControlSet\Control\Session Manager\Environment"; \
    ValueType: expandsz; ValueName: "Path"; ValueData: "{olddata};{app}\bin"; Check: NeedsAddPath(ExpandConstant('{app}\bin'))

[Run]
; setup.ps1 (CLI + service) runs from CurStepChanged below so a failure is reported.
; Launched as the installing user: the daemon binds to the first user that calls it.
Filename: "{app}\octelium-gui.exe"; Description: "Launch Octelium GUI"; Flags: postinstall nowait runasoriginaluser

[UninstallRun]
Filename: "{sys}\sc.exe"; Parameters: "stop OcteliumDaemon"; Flags: runhidden; RunOnceId: "StopSvc"
Filename: "{sys}\sc.exe"; Parameters: "delete OcteliumDaemon"; Flags: runhidden; RunOnceId: "DelSvc"

[UninstallDelete]
Type: filesandordirs; Name: "{app}\bin"

[Code]
function NeedsAddPath(Dir: string): Boolean;
var
  Path: string;
begin
  if not RegQueryStringValue(HKLM, 'SYSTEM\CurrentControlSet\Control\Session Manager\Environment', 'Path', Path) then
    Result := True
  else
    Result := Pos(';' + Uppercase(Dir) + ';', ';' + Uppercase(Path) + ';') = 0;
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  Code: Integer;
begin
  if CurStep <> ssPostInstall then Exit;
  WizardForm.StatusLabel.Caption := 'Installing the Octelium CLI and service...';
  if not Exec('powershell.exe',
      '-NoProfile -ExecutionPolicy Bypass -File "' + ExpandConstant('{app}\setup.ps1') + '" -AppDir "' + ExpandConstant('{app}') + '"',
      '', SW_HIDE, ewWaitUntilTerminated, Code) or (Code <> 0) then
    MsgBox('Installing the Octelium CLI or the OcteliumDaemon service failed (exit code ' + IntToStr(Code) + ').' + #13#10 +
      'See ' + ExpandConstant('{app}\setup.log') + ' for details.', mbError, MB_OK);
end;
