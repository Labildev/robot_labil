$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent $PSScriptRoot
$OllamaExe = Join-Path $RepoRoot "tools\ollama\ollama.exe"
$ModelsDir = Join-Path $RepoRoot "storage\models"

if (-not (Test-Path $OllamaExe)) {
    Write-Error "Ollama portable belum ditemukan di $OllamaExe"
}

New-Item -ItemType Directory -Force -Path $ModelsDir | Out-Null

$env:OLLAMA_MODELS = $ModelsDir
$env:OLLAMA_HOST = "127.0.0.1:11434"
$env:OLLAMA_MAX_LOADED_MODELS = "1"
$env:OLLAMA_KEEP_ALIVE = "2m"
$env:OLLAMA_NUM_PARALLEL = "1"

Write-Host "Starting local Ollama..."
Write-Host "  OLLAMA_MODELS = $env:OLLAMA_MODELS"
Write-Host "  OLLAMA_HOST   = $env:OLLAMA_HOST"

& $OllamaExe serve
