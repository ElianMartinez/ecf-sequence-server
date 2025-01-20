package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"ecf-sequence-server/internal/dbf"
)

// Variables globales para pruebas
var testAPIKey = "test-api-key"

// setupTestService configura un servicio de prueba con el DBF real en la ruta especificada
func setupTestService(t *testing.T) (*apiServerService, func()) {
	t.Helper()

	// Obtiene el directorio actual de este archivo de test
	_, filename, _, _ := runtime.Caller(0)
	currentDir := filepath.Dir(filename)

	// Ajusta la ruta hacia tu archivo real "FAC_PF_M.DBF"
	// En este ejemplo asume que está en ../../DBF/FAC_PF_M.DBF
	dbfPath := filepath.Join(currentDir, "..", "..", "DBF", "FAC_PF_M.DBF")

	// Crear manager
	manager, err := dbf.NewManager(dbfPath)
	if err != nil {
		t.Fatalf("Error creando manager: %v", err)
	}

	// Configurar API key para pruebas
	apiKey = &testAPIKey

	// Crear servicio (apiServerService)
	svc := &apiServerService{
		manager: manager,
		done:    make(chan struct{}),
	}

	// Configurar rutas HTTP usando un mux
	mux := http.NewServeMux()
	setupRoutes(svc, mux)
	svc.server = &http.Server{
		Handler: mux,
	}

	// Función de limpieza para llamar en defer
	cleanup := func() {
		if svc.server != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			svc.server.Shutdown(ctx)
		}
	}

	return svc, cleanup
}

// setupRoutes configura las rutas HTTP para nuestras pruebas
func setupRoutes(svc *apiServerService, mux *http.ServeMux) {
	// Endpoint de salud
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	// Endpoint de tipos
	mux.HandleFunc("/api/tipos", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("X-API-Key") != *apiKey {
			http.Error(w, "No autorizado", http.StatusUnauthorized)
			return
		}
		tipos, err := svc.manager.GetRecordTypes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tipos)
	})

	// Endpoint de secuencias
	mux.HandleFunc("/api/sequence", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("X-API-Key") != *apiKey {
			http.Error(w, "No autorizado", http.StatusUnauthorized)
			return
		}

		var req struct {
			Type string `json:"type"`
			CTA  string `json:"cta"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Solicitud inválida", http.StatusBadRequest)
			return
		}

		if len(req.Type) != 3 {
			http.Error(w, "Tipo de secuencia inválido", http.StatusBadRequest)
			return
		}

		// Si CTA no es "A" o "B", forzamos "A"
		if req.CTA != "A" && req.CTA != "B" {
			req.CTA = "A"
		}

		sequence, seqNum, err := svc.manager.GetSequence(req.Type, req.CTA)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"sequence":       sequence,
			"sequenceNumber": strconv.FormatInt(seqNum, 10),
		})
	})
}

// ----- TESTS -----

func TestHealthEndpoint(t *testing.T) {
	svc, cleanup := setupTestService(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	svc.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status code esperado %d, obtuvimos %d", http.StatusOK, w.Code)
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Error decodificando respuesta: %v", err)
	}

	if result["status"] != "healthy" {
		t.Errorf("Estado esperado 'healthy', obtuvimos '%s'", result["status"])
	}
}

func TestTiposEndpoint(t *testing.T) {
	svc, cleanup := setupTestService(t)
	defer cleanup()

	tests := []struct {
		name       string
		method     string
		apiKey     string
		wantStatus int
	}{
		{
			name:       "sin api key",
			method:     http.MethodGet,
			apiKey:     "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "api key inválida",
			method:     http.MethodGet,
			apiKey:     "invalid-key",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "método no permitido",
			method:     http.MethodPost,
			apiKey:     testAPIKey,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "request válido",
			method:     http.MethodGet,
			apiKey:     testAPIKey,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/tipos", nil)
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}
			w := httptest.NewRecorder()
			svc.server.Handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("[%s] Status code esperado %d, obtuvimos %d", tt.name, tt.wantStatus, w.Code)
			}

			if w.Code == http.StatusOK {
				// Deberíamos obtener una lista de tipos
				var tipos []dbf.ComprobanteTipo
				if err := json.NewDecoder(w.Body).Decode(&tipos); err != nil {
					t.Fatalf("Error decodificando respuesta: %v", err)
				}
				if len(tipos) == 0 {
					t.Error("Se esperaban tipos de comprobantes, pero la respuesta está vacía")
				}
			}
		})
	}
}

func TestSequenceEndpoint(t *testing.T) {
	svc, cleanup := setupTestService(t)
	defer cleanup()

	// Nota: ajusta los valores de "type_" si en tu DBF real tienes B03, E32, etc.
	tests := []struct {
		name       string
		method     string
		apiKey     string
		type_      string
		cta        string
		wantStatus int
	}{
		{
			name:       "sin api key",
			method:     http.MethodPost,
			apiKey:     "",
			type_:      "E32",
			cta:        "A",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "método no permitido",
			method:     http.MethodGet,
			apiKey:     testAPIKey,
			type_:      "E32",
			cta:        "A",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "tipo inexistente en DBF",
			method:     http.MethodPost,
			apiKey:     testAPIKey,
			type_:      "XXX", // length=3, pero no existe => Internal Server Error
			cta:        "A",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "tipo válido, CTA por defecto (A)",
			method:     http.MethodPost,
			apiKey:     testAPIKey,
			type_:      "E32", // Asume que sí existe en tu DBF
			cta:        "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "tipo válido, CTA A explícita",
			method:     http.MethodPost,
			apiKey:     testAPIKey,
			type_:      "E32",
			cta:        "A",
			wantStatus: http.StatusOK,
		},
		{
			name:       "tipo válido, CTA B",
			method:     http.MethodPost,
			apiKey:     testAPIKey,
			type_:      "E32",
			cta:        "B",
			wantStatus: http.StatusOK,
		},
		{
			name:       "CTA inválida => se forzará A",
			method:     http.MethodPost,
			apiKey:     testAPIKey,
			type_:      "E32",
			cta:        "X",
			wantStatus: http.StatusOK,
		},
		{
			name:       "longitud distinta de 3 => Bad Request",
			method:     http.MethodPost,
			apiKey:     testAPIKey,
			type_:      "E3", // length=2
			cta:        "A",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyJSON := fmt.Sprintf(`{"type":"%s","cta":"%s"}`, tt.type_, tt.cta)
			body := bytes.NewBuffer([]byte(bodyJSON))
			req := httptest.NewRequest(tt.method, "/api/sequence", body)
			req.Header.Set("Content-Type", "application/json")
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}

			w := httptest.NewRecorder()
			svc.server.Handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("[%s] Status code esperado %d, obtuvimos %d", tt.name, tt.wantStatus, w.Code)
			}

			// Validar respuesta solo si el status es OK
			if w.Code == http.StatusOK {
				var resp map[string]string
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Error decodificando respuesta: %v", err)
				}

				sequence := resp["sequence"]
				if sequence == "" {
					t.Error("Se esperaba una 'sequence' en la respuesta, pero está vacía")
				}

				seqNumStr := resp["sequenceNumber"]
				if seqNumStr == "" {
					t.Error("Se esperaba un 'sequenceNumber' en la respuesta, pero está vacío")
				}
				// Intentar parsear seqNum
				if _, err := strconv.Atoi(seqNumStr); err != nil {
					t.Errorf("sequenceNumber no es un entero válido: %s", seqNumStr)
				}
			}
		})
	}
}

func TestConcurrentSequenceRequests(t *testing.T) {
	svc, cleanup := setupTestService(t)
	defer cleanup()

	const numRequests = 10
	results := make(chan string, numRequests)
	errors := make(chan error, numRequests)
	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			body := bytes.NewBuffer([]byte(`{"type":"E32","cta":"A"}`)) // Ajusta el tipo según tu DBF
			req := httptest.NewRequest(http.MethodPost, "/api/sequence", body)
			req.Header.Set("X-API-Key", testAPIKey)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			svc.server.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				errors <- fmt.Errorf("status code inesperado: %d", w.Code)
				return
			}

			var resp map[string]string
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				errors <- fmt.Errorf("error decodificando respuesta: %v", err)
				return
			}

			sequence := resp["sequence"]
			if sequence == "" {
				errors <- fmt.Errorf("se esperaba un 'sequence' no vacío")
				return
			}
			results <- sequence
		}()
	}

	// Esperar a que todas las goroutines terminen o timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	sequences := make(map[string]bool)
	timeout := time.After(5 * time.Second)

	for i := 0; i < numRequests; i++ {
		select {
		case err := <-errors:
			if err != nil {
				t.Errorf("Error en request concurrente: %v", err)
			}
		case seq := <-results:
			// Verificamos duplicados
			if sequences[seq] {
				t.Errorf("Secuencia duplicada detectada: %s", seq)
			}
			sequences[seq] = true
		case <-timeout:
			t.Fatal("Timeout esperando resultados")
		}
	}

	select {
	case <-done:
		// OK, todas las goroutines terminaron
	case <-time.After(1 * time.Second):
		t.Error("Timeout esperando que todas las goroutines finalicen")
	}

	if len(sequences) != numRequests {
		t.Errorf("Se esperaban %d secuencias únicas, se obtuvieron %d", numRequests, len(sequences))
	}
}

func TestServiceShutdown(t *testing.T) {
	svc, cleanup := setupTestService(t)
	defer cleanup()

	// Iniciar una goroutine para simular el cierre del servicio
	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		svc.server.Shutdown(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Servicio cerrado correctamente
	case <-time.After(6 * time.Second):
		t.Error("Timeout esperando que el servicio se cierre")
	}
}
