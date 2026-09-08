@{
    RootModule        = 'wt.psm1'
    ModuleVersion     = '0.2.0'
    GUID              = 'b3e5a5b1-8a3a-4e0a-9f9b-6f6a2b0f6c11'
    Author            = 'Worktree Manager'
    Description       = 'Gestiona Git Worktrees para ejecucion paralela de agentes (Copilot CLI + VS Code + Warp).'
    PowerShellVersion = '5.1'
    FunctionsToExport = @(
        # Entrypoint
        'Invoke-Wt', 'Show-WtHelp',
        # Comandos
        'New-WtWorktree', 'Get-WtWorktreeList', 'Open-WtWorktree', 'Remove-WtWorktree',
        'Invoke-WtPrune', 'Invoke-WtPathCommand', 'Invoke-WtDoctor', 'Start-WtConsole',
        'Invoke-WtLock', 'Invoke-WtUnlock',
        'Invoke-WtVersion', 'Invoke-WtClean',
        'Resolve-WtCopyOnCreatePlan', 'Invoke-WtCreateHooks',
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
    CmdletsToExport   = @()
    VariablesToExport = @()
    AliasesToExport   = @()
    PrivateData       = @{
        PSData = @{
            Tags = @('git', 'worktree', 'windows', 'powershell')
        }
    }
}
