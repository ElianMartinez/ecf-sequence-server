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
	"sync"
	"testing"
	"time"

	"ecf-sequence-server/internal/dbf"
)

// Variables globales para pruebas
var testAPIKey = "test-api-key"

// setupTestService configura un servicio de prueba con un DBF temporal
func setupTestService(t *testing.T) (*apiServerService, func()) {
	t.Helper()
	// Get current file directory
	_, filename, _, _ := runtime.Caller(0)
	currentDir := filepath.Dir(filename)

	// Navigate to project root and then to DBF folder
	dbfPath := filepath.Join(currentDir, "..", "..", "DBF", "FAC_PF_M.DBF")
	// Crear manager
	manager, err := dbf.NewManager(dbfPath)
	if err != nil {
		t.Fatalf("Error creando manager: %v", err)
	}

	// Configurar API key para pruebas
	apiKey = &testAPIKey

	// Crear servicio
	svc := &apiServerService{
		manager: manager,
		done:    make(chan struct{}),
	}

	// Configurar rutas HTTP
	mux := http.NewServeMux()
	setupRoutes(svc, mux)
	svc.server = &http.Server{
		Handler: mux,
	}

	// Función de limpieza
	cleanup := func() {
		if svc.server != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			svc.server.Shutdown(ctx)
		}

	}

	return svc, cleanup
}

// setupRoutes configura las rutas HTTP para pruebas
func setupRoutes(svc *apiServerService, mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

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
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Solicitud inválida", http.StatusBadRequest)
			return
		}

		sequence, err := svc.manager.GetSequence(req.Type)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"sequence": sequence})
	})
}

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

	svc, cleanup := setupTestService(t)
	defer cleanup()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/tipos", nil)
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}
			w := httptest.NewRecorder()
			svc.server.Handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Status code esperado %d, obtuvimos %d", tt.wantStatus, w.Code)
			}

			if w.Code == http.StatusOK {
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
	tests := []struct {
		name       string
		method     string
		apiKey     string
		type_      string
		wantStatus int
	}{
		{
			name:       "sin api key",
			method:     http.MethodPost,
			apiKey:     "",
			type_:      "E32",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "método no permitido",
			method:     http.MethodGet,
			apiKey:     testAPIKey,
			type_:      "E32",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "tipo inválido",
			method:     http.MethodPost,
			apiKey:     testAPIKey,
			type_:      "XXX",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "request válido",
			method:     http.MethodPost,
			apiKey:     testAPIKey,
			type_:      "E32",
			wantStatus: http.StatusOK,
		},
	}

	svc, cleanup := setupTestService(t)
	defer cleanup()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := bytes.NewBuffer([]byte(fmt.Sprintf(`{"type":"%s"}`, tt.type_)))
			req := httptest.NewRequest(tt.method, "/api/sequence", body)
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			svc.server.Handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Status code esperado %d, obtuvimos %d", tt.wantStatus, w.Code)
			}

			if w.Code == http.StatusOK {
				var resp map[string]string
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Error decodificando respuesta: %v", err)
				}
				if seq := resp["sequence"]; seq == "" {
					t.Error("Se esperaba una secuencia, pero la respuesta está vacía")
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

			body := bytes.NewBuffer([]byte(`{"type":"E32"}`))
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

			if seq := resp["sequence"]; seq != "" {
				results <- seq
			} else {
				errors <- fmt.Errorf("secuencia vacía en la respuesta")
			}
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
			t.Errorf("Error en request concurrente: %v", err)
		case seq := <-results:
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
		// Todo bien, todas las goroutines terminaron
	case <-time.After(1 * time.Second):
		t.Error("Timeout esperando que todas las goroutines terminen")
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
		// El servicio se cerró correctamente
	case <-time.After(6 * time.Second):
		t.Error("Timeout esperando que el servicio se cierre")
	}
}
