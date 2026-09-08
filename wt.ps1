#requires -Version 5.1
<#
.SYNOPSIS
    Entrypoint del comando wt (Worktree Manager).
.EXAMPLE
    powershell -File wt.ps1 create logging --base origin/develop
#>
[CmdletBinding()]
param([Parameter(ValueFromRemainingArguments)][string[]]$CommandArgs)

Import-Module (Join-Path $PSScriptRoot 'wt.psm1') -Force
Invoke-Wt -Arguments $CommandArgs
exit $LASTEXITCODE
