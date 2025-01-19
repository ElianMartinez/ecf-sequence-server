@echo off
echo Compilando ECF Sequence Server...

:: Establecer variables de entorno para la compilación
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0

:: Crear directorio de salida si no existe
if not exist "build" mkdir build

:: Compilar el servidor principal
echo Compilando servidor...
go build -ldflags="-s -w" -o build/ecf-sequence.exe ./cmd/server

:: Compilar el instalador
echo Compilando instalador...
go build -ldflags="-s -w" -o build/install.exe ./cmd/install

:: Verificar si la compilación fue exitosa
if %ERRORLEVEL% == 0 (
    echo.
    echo Compilación completada exitosamente!
    echo Los ejecutables están en la carpeta 'build':
    echo  - build/ecf-sequence.exe
    echo  - build/install.exe
    echo.
    echo Para instalar el servicio:
    echo 1. Copie los archivos DBF y CDX junto con los ejecutables
    echo 2. Ejecute install.exe como administrador
) else (
    echo.
    echo Error durante la compilación!
)

pause