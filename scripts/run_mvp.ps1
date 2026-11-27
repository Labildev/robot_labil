$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $RepoRoot

$OllamaExe = Join-Path $RepoRoot "tools\ollama\ollama.exe"
$RobotExe = Join-Path $RepoRoot "robot.exe"
$ModelsDir = Join-Path $RepoRoot "storage\models"

if (-not (Test-Path $RobotExe)) {
    throw "robot.exe belum ditemukan. Jalankan: go build -o robot.exe ./cmd/robot"
}
if (-not (Test-Path $OllamaExe)) {
    throw "Ollama portable belum ditemukan di $OllamaExe. Jalankan .\scripts\setup_ollama.ps1"
}

$ollamaProc = $null
$alreadyRunning = $false
try {
    $null = Invoke-RestMethod -Uri "http://127.0.0.1:11434/api/tags" -TimeoutSec 2
    $alreadyRunning = $true
} catch {
    $alreadyRunning = $false
}

if (-not $alreadyRunning) {
    $env:OLLAMA_MODELS = $ModelsDir
    $env:OLLAMA_HOST = "127.0.0.1:11434"
    $env:OLLAMA_MAX_LOADED_MODELS = "1"
    $env:OLLAMA_KEEP_ALIVE = "2m"
    $env:OLLAMA_NUM_PARALLEL = "1"

    Write-Host "Menyalakan Ollama lokal (model di $ModelsDir)..."
    $ollamaProc = Start-Process -FilePath $OllamaExe -ArgumentList "serve" -PassThru -WindowStyle Hidden
    Start-Sleep -Seconds 3
} else {
    Write-Host "Ollama lokal sudah aktif di 127.0.0.1:11434"
}

try {
    Write-Host "Membuka Dashboard Lokal di http://127.0.0.1:8080 ..."
    Start-Process "http://127.0.0.1:8080"
    & $RobotExe serve
}
finally {
    if ($ollamaProc -and -not $ollamaProc.HasExited) {
        Write-Host "Menghentikan Ollama lokal..."
        Stop-Process -Id $ollamaProc.Id -Force
    }
}
