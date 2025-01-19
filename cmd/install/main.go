package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

const windowsServiceScript = `@echo off
echo Installing E-CF Sequence Service...

sc.exe create ECFSequence binPath= "%~dp0sequence-server.exe -dbf \"{{.DBFPath}}\" -port {{.Port}} -key \"{{.APIKey}}\"" start= auto
sc.exe description ECFSequence "E-CF Sequence Generation Service"
sc.exe start ECFSequence

echo Service installed successfully!
pause`

type ServiceConfig struct {
	DBFPath string
	Port    string
	APIKey  string
}

func main() {
	var config ServiceConfig

	// Solicitar información
	fmt.Print("Ruta del archivo DBF (ejemplo: C:\\facturacion\\FAC_PF_M.DBF): ")
	fmt.Scanln(&config.DBFPath)

	fmt.Print("Puerto para el servicio (default: 8080): ")
	fmt.Scanln(&config.Port)
	if config.Port == "" {
		config.Port = "8080"
	}

	fmt.Print("API Key para el servicio: ")
	fmt.Scanln(&config.APIKey)
	if config.APIKey == "" {
		fmt.Println("API Key es requerida")
		os.Exit(1)
	}

	// Validar archivo DBF
	if _, err := os.Stat(config.DBFPath); os.IsNotExist(err) {
		fmt.Printf("Error: El archivo DBF no existe en: %s\n", config.DBFPath)
		os.Exit(1)
	}

	// Validar archivo CDX
	cdxPath := config.DBFPath[:len(config.DBFPath)-4] + ".cdx"
	if _, err := os.Stat(cdxPath); os.IsNotExist(err) {
		fmt.Printf("Error: El archivo CDX no existe en: %s\n", cdxPath)
		os.Exit(1)
	}

	// Obtener directorio del ejecutable
	execPath, err := os.Executable()
	if err != nil {
		fmt.Printf("Error getting executable path: %v\n", err)
		os.Exit(1)
	}

	// Crear archivo de servicio
	servicePath := filepath.Join(filepath.Dir(execPath), "install-service.bat")
	f, err := os.Create(servicePath)
	if err != nil {
		fmt.Printf("Error creating service file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	// Aplicar template
	tmpl := template.Must(template.New("service").Parse(windowsServiceScript))
	if err := tmpl.Execute(f, config); err != nil {
		fmt.Printf("Error writing service file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nArchivos generados exitosamente!")
	fmt.Println("Para instalar el servicio:")
	fmt.Println("1. Ejecute install-service.bat como administrador")
	fmt.Printf("2. El servicio se iniciará automáticamente en el puerto %s\n", config.Port)
	fmt.Println("\nPara probar el servicio:")
	fmt.Printf("curl -X POST http://localhost:%s/api/sequence -H \"X-API-Key: %s\" -H \"Content-Type: application/json\" -d '{\"type\":\"E31\"}'\n",
		config.Port, config.APIKey)
}
