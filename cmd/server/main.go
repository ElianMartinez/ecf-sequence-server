package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"

	"ecf-sequence-server/internal/dbf"
)

// APIResponse estructura para respuestas del API
type APIResponse struct {
	Sequence string `json:"sequence,omitempty"`
	Error    string `json:"error,omitempty"`
}

func main() {
	// Parámetros de línea de comando
	dbfPath := flag.String("dbf", "FAC_PF_M.DBF", "Ruta al archivo DBF")
	port := flag.String("port", "8080", "Puerto para el servidor")
	apiKey := flag.String("key", "", "API Key para autenticación")
	flag.Parse()

	if *apiKey == "" {
		log.Fatal("API Key es requerida. Use -key para especificarla")
	}

	// Inicializar DBF Manager
	manager, err := dbf.NewManager(*dbfPath)
	if err != nil {
		log.Fatalf("Error initializing DBF manager: %v", err)
	}

	// Handler para obtener tipos de comprobantes
	http.HandleFunc("/api/tipos", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
			return
		}

		// Verificar API Key
		if r.Header.Get("X-API-Key") != *apiKey {
			manager.Log(fmt.Sprintf("Intento de acceso no autorizado desde %s", r.RemoteAddr))
			http.Error(w, "No autorizado", http.StatusUnauthorized)
			return
		}

		tipos, err := manager.GetRecordTypes()
		if err != nil {
			manager.Log(fmt.Sprintf("Error obteniendo tipos: %v", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tipos)
	})

	// Handler para solicitudes de secuencia
	http.HandleFunc("/api/sequence", func(w http.ResponseWriter, r *http.Request) {
		// Verificar método
		if r.Method != http.MethodPost {
			http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
			return
		}

		// Verificar API Key
		if r.Header.Get("X-API-Key") != *apiKey {
			manager.Log(fmt.Sprintf("Intento de acceso no autorizado desde %s", r.RemoteAddr))
			http.Error(w, "No autorizado", http.StatusUnauthorized)
			return
		}

		// Decodificar solicitud
		var req struct {
			Type string `json:"type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Solicitud inválida", http.StatusBadRequest)
			return
		}

		// Validar tipo de secuencia
		if !strings.HasPrefix(req.Type, "E") || len(req.Type) != 3 {
			http.Error(w, "Tipo de secuencia inválido", http.StatusBadRequest)
			return
		}

		// Obtener secuencia
		sequence, err := manager.GetSequence(req.Type)
		if err != nil {
			manager.Log(fmt.Sprintf("Error generando secuencia: %v", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Responder
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(APIResponse{Sequence: sequence})
	})

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	// Iniciar servidor
	addr := ":" + *port
	manager.Log(fmt.Sprintf("Servidor iniciado en puerto %s", *port))
	log.Printf("Servidor iniciado en puerto %s", *port)
	log.Fatal(http.ListenAndServe(addr, nil))
}
