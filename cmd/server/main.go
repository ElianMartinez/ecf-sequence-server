package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"

	"ecf-sequence-server/internal/dbf"
)

const serviceName = "ECFSequence"
const displayName = "ECF Sequence Service"
const serviceDescription = "Servicio de generación de secuencias E-CF"

// serviceFlags son los flags que parseamos desde la línea de comandos.
var (
	dbfPath = flag.String("dbf", "", "Ruta al archivo DBF")
	port    = flag.String("port", "8080", "Puerto para el servidor")
	apiKey  = flag.String("key", "", "API Key para autenticación")
	debugF  = flag.Bool("debug", false, "Ejecutar en modo debug (no como servicio)")
)

// apiServerService es el "contexto de servicio" que implementa svc.Handler
type apiServerService struct {
	manager *dbf.Manager
	server  *http.Server
	done    chan struct{}
}

// Execute es donde el SCM (Service Control Manager) interactúa con el servicio.
// Acá escuchamos señales de Start, Stop, Pause, etc. y hacemos lo necesario.
func (m *apiServerService) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (svcSpecificEC bool, exitCode uint32) {

	// cmdsAccepted indica qué señales vamos a aceptar:
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue

	// Avisamos al SCM que estamos "iniciando".
	s <- svc.Status{State: svc.StartPending}

	// Iniciar la goroutine del servidor HTTP
	go m.runHTTPServer()

	// Avisamos que ya estamos "corriendo" y aceptamos las señales indicadas.
	s <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	// Bucle principal de escucha de señales
loop:
	for {
		select {
		// No tenemos un "tick" en este caso, así que solo escuchamos señales del SCM
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				// El SCM pregunta "¿cómo estás?" => respondemos con estado actual
				s <- c.CurrentStatus

			case svc.Stop, svc.Shutdown:
				// Nos ordenan detener => paramos el servidor y salimos
				log.Print("Recibida señal de STOP/SHUTDOWN, cerrando el servidor.")
				m.stopHTTPServer()
				break loop

			case svc.Pause:
				// Opcionalmente, podrías pausar el HTTP server, etc.
				s <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}

			case svc.Continue:
				// Reactivar si se pausó
				s <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

			default:
				log.Printf("Recibida señal no esperada: %v", c.Cmd)
			}
		}
	}

	// Avisar al SCM que estamos en proceso de detener
	s <- svc.Status{State: svc.StopPending}

	// Retornamos (fin de Execute => fin de servicio)
	return false, 0
}

// runHTTPServer arranca el servidor HTTP en una goroutine.
func (m *apiServerService) runHTTPServer() {
	mux := http.NewServeMux()

	// Handlers
	mux.HandleFunc("/api/tipos", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
			return
		}
		// Verificar API Key
		if r.Header.Get("X-API-Key") != *apiKey {
			m.manager.Log(fmt.Sprintf("Intento de acceso no autorizado desde %s", r.RemoteAddr))
			http.Error(w, "No autorizado", http.StatusUnauthorized)
			return
		}
		tipos, err := m.manager.GetRecordTypes()
		if err != nil {
			m.manager.Log(fmt.Sprintf("Error obteniendo tipos: %v", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tipos)
	})

	mux.HandleFunc("/api/sequence", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("X-API-Key") != *apiKey {
			m.manager.Log(fmt.Sprintf("Intento de acceso no autorizado desde %s", r.RemoteAddr))
			http.Error(w, "No autorizado", http.StatusUnauthorized)
			return
		}
		var req struct {
			Type string `json:"type"`
			CTA  string `json:"cta"` // A: Default  - B: Cuenta Izquierda
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Solicitud inválida", http.StatusBadRequest)
			return
		}
		if len(req.Type) != 3 {
			http.Error(w, "Tipo de secuencia inválido", http.StatusBadRequest)
			return
		}

		//validar CTA y is es B o A y si no hay valor que sea A por default
		if req.CTA == "" || (req.CTA != "A" && req.CTA != "B") {
			req.CTA = "A"
		}

		sequence, num, err := m.manager.GetSequence(req.Type, req.CTA)
		if err != nil {
			m.manager.Log(fmt.Sprintf("Error generando secuencia: %v", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"sequence": sequence, "sequenceNumber": fmt.Sprintf("%d", num)})
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	m.server = &http.Server{
		Addr:    ":" + *port,
		Handler: mux,
	}

	log.Printf("Servidor iniciado en puerto %s", *port)
	m.manager.Log(fmt.Sprintf("Servidor iniciado en puerto %s", *port))

	// Bloqueante hasta que se cierre
	if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("Error en servidor HTTP: %v", err)
	}
	close(m.done) // señal de que hemos salido
}

// stopHTTPServer detiene el servidor HTTP de forma ordenada.
func (m *apiServerService) stopHTTPServer() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.server.Shutdown(ctx); err != nil {
		log.Printf("Error al cerrar el servidor: %v", err)
	}
	<-m.done // esperamos a que la goroutine de ListenAndServe termine
}

// runService permite ejecutar el servicio en modo real (SCM) o debug.
func runService(name string, isDebug bool, manager *dbf.Manager) {
	svcHandler := &apiServerService{
		manager: manager,
		done:    make(chan struct{}),
	}
	if isDebug {
		// Modo consola normal (debug.Run => no necesita instalarse en Windows)
		err := debug.Run(name, svcHandler)
		if err != nil {
			log.Fatalf("Error corriendo en modo debug: %v", err)
		}
	} else {
		// Modo servicio normal => Interactúa con el SCM
		err := svc.Run(name, svcHandler)
		if err != nil {
			log.Fatalf("Error corriendo como servicio: %v", err)
		}
	}
}

func main() {
	// Parsear flags: -dbf, -port, -key, -debug
	flag.Parse()

	// Validar que tengamos la dbf y la key
	if *dbfPath == "" || *apiKey == "" {
		// Nota: en modo debug podemos permitir no tenerlos, pero idealmente no.
		// Si quieres forzar, hazlo:
		fmt.Println("Uso: ecf-sequence.exe -dbf=C:\\path\\FAC_PF_M.DBF -key=XYZ [opciones]")
		os.Exit(1)
	}

	// Abrir archivo de log (opcional) o logs a stdout
	f, err := os.OpenFile("ecf-sequence.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("No se pudo abrir ecf-sequence.log: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)

	// Crear Manager DBF
	manager, err := dbf.NewManager(*dbfPath)
	if err != nil {
		log.Fatalf("Error inicializando DBF manager: %v", err)
	}

	// Iniciar el servicio (o debug)
	runService(serviceName, *debugF, manager)
}
