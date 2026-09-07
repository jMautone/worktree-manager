@{
    # Arranca con las reglas por defecto de PSScriptAnalyzer. Si una regla genera
    # ruido en una funcion puntual que no cambia estado (ej. PSUseShouldProcess-
    # ForStateChangingFunctions en un constructor puro), se excluye por funcion
    # con SuppressMessageAttribute, nunca globalmente aca.
    #
    # Excepcion: dos reglas que chocan de raiz con el diseno del CLI, no con casos
    # puntuales, se excluyen globalmente:
    #   - PSAvoidUsingWriteHost: Common.ps1 declara ser "el unico punto donde se
    #     escribe a consola" (por diseno, no por descuido) y los scripts de
    #     tests/ imprimen resultados con color por la misma razon. Anotar cada
    #     Write-Host de un CLI/test-runner no aporta nada que el comentario de
    #     Common.ps1 no diga ya.
    #   - PSUseSingularNouns: funciones como Get-WtWorktrees o Get-WtRepoDirs
    #     devuelven colecciones; el plural es mas preciso que el singular y
    #     renombrarlas no cambia comportamiento, solo rompe la superficie publica.
    Severity     = @('Error', 'Warning')
    ExcludeRules = @('PSAvoidUsingWriteHost', 'PSUseSingularNouns')
}
