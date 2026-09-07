# Changelog

Formato basado en [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Este archivo se actualiza por fase, siguiendo la ejecucion de
[PLAN-CORRECCIONES.md](PLAN-CORRECCIONES.md).

## [Unreleased]

### Fase 0 — Red de contencion

- **M6**: `WT_CONFIG_ONLY=1` aisla la config efectiva a `defaults < WT_CONFIG`,
  ignorando la config global del usuario y el `.wt.json` del repo. La suite E2E
  ahora corre con las 11 claves explicitas, sin depender de la maquina.
- **M9**: se agrega `wt.psd1` (manifiesto del modulo, version `0.1.0`), el
  comando `wt version`, `.gitignore`, `LICENSE` (MIT), este `CHANGELOG.md`,
  CI en GitHub Actions (`windows-latest`, matriz `powershell`/`pwsh`) y
  `PSScriptAnalyzerSettings.psd1` con las reglas por defecto.

### Fase 1 — Riesgo alto

- **A1**: el `.wt.json` del repo ahora pasa por una lista blanca
  (`Get-WtRepoConfigAllowedKeys` / `Select-WtRepoConfigKeys`, funciones puras):
  solo `worktreeRootTemplate`, `defaultBase`, `branchPrefix` y
  `fetchBeforeCreate`. Cualquier otra clave (`editor`, `warpPath`, ...) se
  descarta con un warning que nombra el archivo y la clave, para que un
  `.wt.json` de un repo clonado no pueda decidir que ejecutable lanza
  `wt open`.
