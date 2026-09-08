#requires -Version 5.1
<#
.SYNOPSIS
    Worktree Manager - gestiona Git Worktrees para ejecucion paralela de agentes.
.DESCRIPTION
    Este archivo solo carga las capas de src/ y define la superficie publica.
    El orden de carga refleja la direccion de las dependencias:
    Common -> Config/Repo -> Workspace/Launch -> Commands -> Console/Cli -> Completion.
    Importar el modulo no produce efectos secundarios (Register-WtCompletion se llama
    aparte, desde el bloque del perfil que instala install.ps1).
#>

Set-StrictMode -Version Latest

foreach ($file in @('Common', 'Config', 'Repo', 'Workspace', 'Launch', 'Commands', 'Console', 'Cli', 'Completion')) {
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
    'Resolve-WtCopyOnCreatePlan', 'Invoke-WtCreateHooks',
    'Invoke-WtStatus', 'Get-WtStatusRows', 'Get-WtStatusEntry',
    'Invoke-WtExec', 'Invoke-WtEach', 'Get-WtEachPlan', 'Invoke-WtCommandLine',
    'Invoke-WtSync', 'Get-WtSyncPlan',
    # Workspace
    'Get-WtRepoList', 'Get-WtRepoDirs', 'Invoke-WtCd', 'Resolve-WtRepoDir',
    'Resolve-WtRepoContext', 'Open-WtFolder', 'Get-WtReposRoot', 'Get-WtReposDepth',
    'Resolve-WtRepoOrWorktreeOwner', 'Find-WtReposOwningWorktree',
    # Config
    'Get-WtConfig', 'Get-WtConfigFilePath', 'Set-WtConfigValue', 'Invoke-WtConfigCommand',
    'Get-WtDefaultConfig', 'Merge-WtConfig', 'Clear-WtConfigCache',
    'Get-WtRepoConfigAllowedKeys', 'Select-WtRepoConfigKeys',
    'Test-WtConfigValue', 'Assert-WtConfigValue', 'Test-WtWorktreeRootTemplateNeedsRepoToken', 'Get-WtKnownWarpColors',
    'ConvertTo-WtConfigValue', 'ConvertTo-WtConfigDisplayValue',
    # Autocompletado
    'Get-WtCompletion', 'Register-WtCompletion', 'Get-WtCommandCompletionNames', 'Get-WtConfigValueCandidates',
    # Repo / worktrees
    'Find-WtMainRoot', 'Get-WtCurrentRoot', 'Get-WtWorktrees', 'Get-WtWorktreesCached', 'Clear-WtWorktreesCache', 'Resolve-WtWorktree',
    'Test-WtWorktreeMatchesName',
    'ConvertFrom-WtWorktreePorcelain', 'Get-WtWorktreePath', 'ConvertFrom-WtStatusPorcelainV2',
    # Warp / apertura
    'Write-WtAgentTabConfig', 'New-WtWarpTabConfigContent', 'Open-WtAgentInWarp',
    'Open-WtTerminalInWarp', 'Test-WtWarpSameWindow', 'Test-WtWarpRunning', 'Open-WtEditor',
    'Get-WtWindowsTerminalArgs', 'Open-WtAgentInWindowsTerminal',
    'Get-WtTabConfigDir', 'Get-WtTabConfigFileName', 'Get-WtTabConfigDirectory',
    'Remove-WtWorktreeTabConfigs', 'Get-WtAgentCommands',
    # Utilidades reutilizables / testeables
    'ConvertFrom-WtArgs', 'Get-WtOpenPlan', 'Get-WtCreateOpenPlan', 'Get-WtWorktreeRows', 'Invoke-WtGit',
    'Invoke-WtProcess', 'Get-WtPathHash', 'Set-WtDryRun', 'Test-WtDryRun', 'Start-WtProcess',
    'ConvertTo-WtFullPath', 'Test-WtPathEquals', 'Test-WtPathIsUnder', 'Test-WtWorktreeName',
    'Get-WtCommandSource', 'Find-WtRepoConfigFile', 'Read-WtGitDirPointer', 'ConvertTo-WtReposRootList',
    'ConvertTo-WtSafeFileName', 'ConvertTo-WtJson'
)
