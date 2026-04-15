Unicode true

####
## Custom Wails v2 NSIS installer script for ec2hosts.
##
## Template conventions from Wails (see the default
## wailsapp/wails/v2/pkg/buildassets template) are preserved. The custom
## bits are:
##
##   1. Bundle the CLI binary (ec2hosts.exe) alongside the GUI, so the
##      internal/elevate helper can re-exec it with UAC to write the
##      hosts file.
##   2. Seed %APPDATA%\ec2hosts\config.yaml from config.yaml.example on
##      first install. Preserved on reinstall/upgrade.
##
## Build contract from the release workflow:
##   - The Wails CLI produces the GUI at build/bin/ec2hosts-gui-amd64.exe
##     and invokes makensis with -DARG_WAILS_AMD64_BINARY pointing there.
##   - The workflow pre-copies the CLI binary into build/bin/ec2hosts.exe
##     so the File directive below resolves.
##   - config.yaml.example is copied into build/windows/installer/ so
##     the File directive has a stable relative path regardless of cwd.
####

!define PRODUCT_EXECUTABLE "${INFO_PROJECTNAME}.exe"
!define UNINST_KEY_NAME "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"

!include "wails_tools.nsh"

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
ShowInstDetails show
ShowUnInstDetails show

Page directory
Page instfiles

UninstPage uninstConfirm
UninstPage instfiles

Function .onInit
    !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext

    # Ensures WebView2 runtime is available (downloaded on demand).
    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    # Installs the Wails GUI binary (ec2hosts-gui.exe).
    !insertmacro wails.files

    # Bundle the CLI binary. The release workflow copies the freshly
    # built ec2hosts.exe into build/bin/ prior to running wails build
    # -nsis, so this path resolves.
    File "..\..\bin\ec2hosts.exe"

    # Seed %APPDATA%\ec2hosts\config.yaml from the example, but only if
    # the user does not already have one (reinstalls must not clobber
    # real configuration).
    SetShellVarContext current
    CreateDirectory "$APPDATA\ec2hosts"
    IfFileExists "$APPDATA\ec2hosts\config.yaml" SkipConfigSeed 0
        SetOutPath "$APPDATA\ec2hosts"
        File "/oname=config.yaml" "config.yaml.example"
        SetOutPath $INSTDIR
    SkipConfigSeed:

    # Start menu + desktop shortcuts point to the GUI binary.
    CreateShortcut  "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut  "$DESKTOP\${INFO_PRODUCTNAME}.lnk"    "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    # WebView2 profile (cache, cookies) for this app.
    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}"

    # Intentionally NOT removing $APPDATA\ec2hosts\ — it contains the
    # user's config.yaml. If they really want it gone they can delete
    # it manually.

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.deleteUninstaller
SectionEnd
