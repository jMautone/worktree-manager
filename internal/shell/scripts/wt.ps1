# wt shell integration for PowerShell 7.4 or later. Load it from $PROFILE with:
#   Invoke-Expression (& git-wt shell init pwsh | Out-String)
#
# wt runs git-wt with its arguments and moves the shell to the directory the
# binary writes to the file named by WT_DIRECTIVE_CD_FILE. Both variables
# are set for that run only and removed afterwards. $global:__WtPreviousDir
# is a shell variable, not an environment one, so a child shell starts with
# no previous directory. A function wins over wt.exe (Windows Terminal) on
# the PATH; type wt.exe to open Windows Terminal.
function global:wt {
    # 7.2 drops empty arguments with the Legacy mode.
    $PSNativeCommandArgumentPassing = 'Standard'
    $PSNativeCommandUseErrorActionPreference = $false
    $exe = Get-Command -Name git-wt -CommandType Application -ErrorAction Stop | Select-Object -First 1
    $tmp = [IO.Path]::GetTempFileName()
    $savedPrevious = $env:WT_PREVIOUS_DIR
    try {
        $env:WT_DIRECTIVE_CD_FILE = $tmp
        $env:WT_PREVIOUS_DIR = $global:__WtPreviousDir
        & $exe @args
        $dest = [IO.File]::ReadAllText($tmp)
        if ($dest) {
            $from = (Get-Location -PSProvider FileSystem).ProviderPath
            Set-Location -LiteralPath $dest
            if ($?) {
                $global:__WtPreviousDir = $from
            } elseif ($global:LASTEXITCODE -eq 0) {
                $global:LASTEXITCODE = 1
            }
        }
    } finally {
        $env:WT_DIRECTIVE_CD_FILE = $null
        $env:WT_PREVIOUS_DIR = $savedPrevious
        Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue
    }
}
