@echo off
echo Compilando ECF Sequence Server...

set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0

if not exist "build" mkdir build

echo Compilando servidor (ecf-sequence.exe)...
go build -o build/ecf-sequence.exe -ldflags="-s -w" ./cmd/server

echo Compilando instalador (install.exe)...
go build -o build/install.exe -ldflags="-s -w" ./cmd/install

if %ERRORLEVEL% == 0 (
    echo.
    echo Compilacion completada exitosamente!
    echo Ejecutables en carpeta 'build':
    echo   - ecf-sequence.exe (el servicio/servidor)
    echo   - install.exe (instalador del servicio)
) else (
    echo.
    echo Error durante la compilacion!
)

pause
