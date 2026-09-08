@{
    RootModule        = 'wt.psm1'
    ModuleVersion     = '0.1.0'
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
        'Invoke-WtVersion',
        # Workspace
        'Get-WtRepoList', 'Get-WtRepoDirs', 'Invoke-WtCd', 'Resolve-WtRepoDir',
        'Resolve-WtRepoContext', 'Open-WtFolder',
        # Config
        'Get-WtConfig', 'Get-WtConfigFilePath', 'Set-WtConfigValue', 'Invoke-WtConfigCommand',
        'Get-WtDefaultConfig', 'Merge-WtConfig', 'Clear-WtConfigCache',
        'Get-WtRepoConfigAllowedKeys', 'Select-WtRepoConfigKeys',
        # Repo / worktrees
        'Find-WtMainRoot', 'Get-WtCurrentRoot', 'Get-WtWorktrees', 'Resolve-WtWorktree',
        'ConvertFrom-WtWorktreePorcelain', 'Get-WtWorktreePath',
        # Warp / apertura
        'Write-WtAgentTabConfig', 'New-WtWarpTabConfigContent', 'Open-WtAgentInWarp',
        'Open-WtTerminalInWarp', 'Test-WtWarpSameWindow', 'Test-WtWarpRunning', 'Open-WtEditor',
        # Utilidades reutilizables / testeables
        'ConvertFrom-WtArgs', 'Get-WtOpenPlan', 'Get-WtCreateOpenPlan', 'Get-WtWorktreeRows', 'Invoke-WtGit',
        'Invoke-WtProcess',
        'ConvertTo-WtFullPath', 'Test-WtPathEquals', 'Test-WtPathIsUnder', 'Test-WtWorktreeName',
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
