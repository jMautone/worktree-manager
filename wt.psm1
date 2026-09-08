#requires -Version 5.1
<#
.SYNOPSIS
    Worktree Manager - gestiona Git Worktrees para ejecucion paralela de agentes.
.DESCRIPTION
    Este archivo solo carga las capas de src/ y define la superficie publica.
    El orden de carga refleja la direccion de las dependencias:
    Common -> Config/Repo -> Workspace/Launch -> Commands -> Console/Cli.
    Importar el modulo no produce efectos secundarios.
#>

Set-StrictMode -Version Latest

foreach ($file in @('Common', 'Config', 'Repo', 'Workspace', 'Launch', 'Commands', 'Console', 'Cli')) {
    . (Join-Path $PSScriptRoot ("src\{0}.ps1" -f $file))
}

Export-ModuleMember -Function @(
    # Entrypoint
    'Invoke-Wt', 'Show-WtHelp',
    # Comandos
    'New-WtWorktree', 'Get-WtWorktreeList', 'Open-WtWorktree', 'Remove-WtWorktree',
    'Invoke-WtPrune', 'Invoke-WtPathCommand', 'Invoke-WtDoctor', 'Start-WtConsole',
    'Invoke-WtLock', 'Invoke-WtUnlock',
    'Invoke-WtVersion', 'Invoke-WtClean',
    # Workspace
    'Get-WtRepoList', 'Get-WtRepoDirs', 'Invoke-WtCd', 'Resolve-WtRepoDir',
    'Resolve-WtRepoContext', 'Open-WtFolder', 'Get-WtReposRoot', 'Get-WtReposDepth',
    'Resolve-WtRepoOrWorktreeOwner', 'Find-WtReposOwningWorktree',
    # Config
    'Get-WtConfig', 'Get-WtConfigFilePath', 'Set-WtConfigValue', 'Invoke-WtConfigCommand',
    'Get-WtDefaultConfig', 'Merge-WtConfig', 'Clear-WtConfigCache',
    'Get-WtRepoConfigAllowedKeys', 'Select-WtRepoConfigKeys',
    'Test-WtConfigValue', 'Assert-WtConfigValue', 'Test-WtWorktreeRootTemplateNeedsRepoToken', 'Get-WtKnownWarpColors',
    # Repo / worktrees
    'Find-WtMainRoot', 'Get-WtCurrentRoot', 'Get-WtWorktrees', 'Clear-WtWorktreesCache', 'Resolve-WtWorktree',
    'Test-WtWorktreeMatchesName',
    'ConvertFrom-WtWorktreePorcelain', 'Get-WtWorktreePath',
    # Warp / apertura
    'Write-WtAgentTabConfig', 'New-WtWarpTabConfigContent', 'Open-WtAgentInWarp',
    'Open-WtTerminalInWarp', 'Test-WtWarpSameWindow', 'Test-WtWarpRunning', 'Open-WtEditor',
    'Get-WtTabConfigDir', 'Get-WtTabConfigFileName', 'Get-WtTabConfigDirectory',
    'Remove-WtWorktreeTabConfigs', 'Get-WtAgentCommands',
    # Utilidades reutilizables / testeables
    'ConvertFrom-WtArgs', 'Get-WtOpenPlan', 'Get-WtCreateOpenPlan', 'Get-WtWorktreeRows', 'Invoke-WtGit',
    'Invoke-WtProcess', 'Get-WtPathHash',
    'ConvertTo-WtFullPath', 'Test-WtPathEquals', 'Test-WtPathIsUnder', 'Test-WtWorktreeName',
    'Get-WtCommandSource', 'Find-WtRepoConfigFile', 'Read-WtGitDirPointer', 'ConvertTo-WtReposRootList',
    'ConvertTo-WtSafeFileName', 'ConvertTo-WtJson'
)
