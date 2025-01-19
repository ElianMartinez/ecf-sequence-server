package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName = "ECFSequence"
	displayName = "ECF Sequence Service"
	description = "Servicio de generación de secuencias E-CF"
)

func main() {
	// Verificar si se ejecuta como administrador
	isAdmin, err := isAdministrator()
	if err != nil || !isAdmin {
		fmt.Println("Este programa debe ejecutarse como administrador")
		fmt.Println("Por favor, clic derecho -> Ejecutar como administrador")
		pressEnterToContinue()
		return
	}

	// Solicitar información
	var dbfPath, port, apiKey string

	fmt.Print("Ruta del archivo DBF (ejemplo: C:\\facturacion\\FAC_PF_M.DBF): ")
	fmt.Scanln(&dbfPath)

	// Validar archivo DBF y CDX
	if err := validateFiles(dbfPath); err != nil {
		fmt.Printf("Error: %v\n", err)
		pressEnterToContinue()
		return
	}

	fmt.Print("Puerto para el servicio (default: 8080): ")
	fmt.Scanln(&port)
	if port == "" {
		port = "8080"
	}

	fmt.Print("API Key para el servicio: ")
	fmt.Scanln(&apiKey)
	if apiKey == "" {
		fmt.Println("API Key es requerida")
		pressEnterToContinue()
		return
	}

	// Obtener ruta del ejecutable del servicio
	exePath, err := getServiceExecutablePath()
	if err != nil {
		fmt.Printf("Error obteniendo ruta del ejecutable: %v\n", err)
		pressEnterToContinue()
		return
	}

	// Construir comando del servicio
	cmd := fmt.Sprintf(`"%s" -dbf "%s" -port %s -key "%s"`, exePath, dbfPath, port, apiKey)

	fmt.Println("\nInstalando servicio...")

	// Instalar el servicio
	if err := installService(cmd); err != nil {
		fmt.Printf("Error instalando el servicio: %v\n", err)
		pressEnterToContinue()
		return
	}

	// Verificar estado del servicio
	fmt.Println("\nVerificando estado del servicio...")
	if err := verifyServiceStatus(); err != nil {
		fmt.Printf("Error verificando servicio: %v\n", err)
		pressEnterToContinue()
		return
	}

	fmt.Println("\nServicio instalado exitosamente!")
	fmt.Println("\nDetalles del servicio:")
	fmt.Printf("Nombre: %s\n", serviceName)
	fmt.Printf("Archivo ejecutable: %s\n", exePath)
	fmt.Printf("Puerto: %s\n", port)
	fmt.Printf("Archivos DBF: %s\n", dbfPath)

	fmt.Println("\nPara probar el servicio, ejecute:")
	fmt.Printf("curl -X POST http://localhost:%s/api/sequence -H \"Content-Type: application/json\" -H \"X-API-Key: %s\" -d \"{\\\"type\\\":\\\"E31\\\"}\"\n",
		port, apiKey)

	fmt.Println("\nPara verificar el estado del servicio:")
	fmt.Printf("sc query %s\n", serviceName)

	fmt.Println("\nPara detener el servicio:")
	fmt.Printf("sc stop %s\n", serviceName)

	fmt.Println("\nPara iniciar el servicio:")
	fmt.Printf("sc start %s\n", serviceName)

	pressEnterToContinue()
}

func isAdministrator() (bool, error) {
	_, err := mgr.Connect()
	if err == nil {
		return true, nil
	}
	return false, err
}

func validateFiles(dbfPath string) error {
	if _, err := os.Stat(dbfPath); os.IsNotExist(err) {
		return fmt.Errorf("el archivo DBF no existe en: %s", dbfPath)
	}

	cdxPath := strings.TrimSuffix(dbfPath, ".DBF") + ".CDX"
	if _, err := os.Stat(cdxPath); os.IsNotExist(err) {
		return fmt.Errorf("el archivo CDX no existe en: %s", cdxPath)
	}

	return nil
}

func getServiceExecutablePath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(execPath)
	servicePath := filepath.Join(dir, "ecf-sequence.exe")

	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		return "", fmt.Errorf("no se encuentra ecf-sequence.exe en: %s", dir)
	}

	return servicePath, nil
}

func installService(cmd string) error {
	// Conectar al administrador de servicios
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("error conectando al service manager: %v", err)
	}
	defer m.Disconnect()

	// Intentar abrir el servicio si existe
	s, err := m.OpenService(serviceName)
	if err == nil {
		// El servicio existe, intentar detenerlo primero
		status, _ := s.Query()
		if status.State != svc.Stopped {
			s.Control(svc.Stop)
			// Esperar a que se detenga
			for i := 0; i < 10; i++ {
				time.Sleep(time.Second)
				status, _ = s.Query()
				if status.State == svc.Stopped {
					break
				}
			}
		}

		// Eliminar el servicio existente
		err = s.Delete()
		s.Close()
		if err != nil {
			return fmt.Errorf("error eliminando servicio existente: %v", err)
		}

		// Esperar un momento para asegurar que el servicio se eliminó
		time.Sleep(2 * time.Second)
	}

	// Crear el nuevo servicio con configuración específica
	config := mgr.Config{
		DisplayName:      displayName,
		StartType:        mgr.StartAutomatic,
		Description:      description,
		ServiceType:      windows.SERVICE_WIN32_OWN_PROCESS,
		ServiceStartName: "NT AUTHORITY\\SYSTEM", // Ejecutar como LocalSystem
		ErrorControl:     mgr.ErrorNormal,
	}

	s, err = m.CreateService(serviceName, cmd, config)
	if err != nil {
		return fmt.Errorf("error creando servicio: %v", err)
	}
	defer s.Close()

	// Configurar acciones de recuperación
	recovery := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: time.Minute},
		{Type: mgr.ServiceRestart, Delay: 2 * time.Minute},
		{Type: mgr.ServiceRestart, Delay: 5 * time.Minute},
	}
	if err := s.SetRecoveryActions(recovery, uint32(86400)); err != nil {
		fmt.Printf("Advertencia: no se pudo configurar la recuperación automática: %v\n", err)
	}

	// Iniciar el servicio
	if err := s.Start(); err != nil {
		return fmt.Errorf("error iniciando servicio: %v", err)
	}

	return nil
}

func verifyServiceStatus() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("no se puede abrir el servicio: %v", err)
	}
	defer s.Close()

	// Esperar hasta 10 segundos para que el servicio inicie
	for i := 0; i < 10; i++ {
		status, err := s.Query()
		if err != nil {
			return fmt.Errorf("error consultando estado: %v", err)
		}

		switch status.State {
		case svc.Running:
			fmt.Println("El servicio está ejecutándose correctamente")
			return nil
		case svc.StartPending:
			fmt.Println("El servicio está iniciando...")
			time.Sleep(time.Second)
			continue
		default:
			return fmt.Errorf("el servicio está en estado inesperado: %d", status.State)
		}
	}

	return fmt.Errorf("tiempo de espera agotado esperando que el servicio inicie")
}

func pressEnterToContinue() {
	fmt.Println("\nPresione Enter para continuar...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}
