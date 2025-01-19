package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

// Configuración de la prueba
type Config struct {
	URL           string
	APIKey        string
	TotalRequests int
	Concurrent    int
	Type          string
}

// Respuesta del servidor
type Response struct {
	Sequence string `json:"sequence"`
	Error    string `json:"error,omitempty"`
}

// Resultado de un request individual
type Result struct {
	Sequence   string
	StatusCode int
	Duration   time.Duration
	Error      error
	StartTime  time.Time
	WorkerID   int
}

// Estadísticas globales
type Stats struct {
	TotalRequests     int
	SuccessfulReqs    int
	FailedReqs        int
	DuplicateSeqs     int
	MinDuration       time.Duration
	MaxDuration       time.Duration
	AvgDuration       time.Duration
	TotalDuration     time.Duration
	RequestsPerSecond float64
	UniqueSequences   map[string][]int // Mapa de secuencia a lista de worker IDs
	mu                sync.Mutex
}

func newStats() *Stats {
	return &Stats{
		MinDuration:     time.Hour, // Valor inicial alto para comparar
		UniqueSequences: make(map[string][]int),
	}
}

func (s *Stats) addResult(r Result) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalRequests++
	if r.Error == nil && r.StatusCode == http.StatusOK {
		s.SuccessfulReqs++
		s.TotalDuration += r.Duration

		if r.Duration < s.MinDuration {
			s.MinDuration = r.Duration
		}
		if r.Duration > s.MaxDuration {
			s.MaxDuration = r.Duration
		}

		if r.Sequence != "" {
			s.UniqueSequences[r.Sequence] = append(s.UniqueSequences[r.Sequence], r.WorkerID)
		}
	} else {
		s.FailedReqs++
	}
}

func (s *Stats) calculateStats(totalDuration time.Duration) {
	if s.SuccessfulReqs > 0 {
		s.AvgDuration = s.TotalDuration / time.Duration(s.SuccessfulReqs)
		s.RequestsPerSecond = float64(s.SuccessfulReqs) / totalDuration.Seconds()
	}

	// Contar duplicados
	for _, workers := range s.UniqueSequences {
		if len(workers) > 1 {
			s.DuplicateSeqs++
		}
	}
}

func (s *Stats) print() {
	fmt.Printf("\nResultados de la prueba:\n")
	fmt.Printf("====================\n")
	fmt.Printf("Total de requests: %d\n", s.TotalRequests)
	fmt.Printf("Requests exitosos: %d\n", s.SuccessfulReqs)
	fmt.Printf("Requests fallidos: %d\n", s.FailedReqs)
	fmt.Printf("Secuencias únicas: %d\n", len(s.UniqueSequences))
	fmt.Printf("Secuencias duplicadas: %d\n", s.DuplicateSeqs)
	fmt.Printf("Duración mínima: %v\n", s.MinDuration)
	fmt.Printf("Duración máxima: %v\n", s.MaxDuration)
	fmt.Printf("Duración promedio: %v\n", s.AvgDuration)
	fmt.Printf("Requests por segundo: %.2f\n", s.RequestsPerSecond)

	// Mostrar duplicados si existen
	if s.DuplicateSeqs > 0 {
		fmt.Printf("\nSecuencias duplicadas:\n")
		for seq, workers := range s.UniqueSequences {
			if len(workers) > 1 {
				fmt.Printf("Secuencia %s generada por workers: %v\n", seq, workers)
			}
		}
	}

	// Mostrar rango de secuencias
	if len(s.UniqueSequences) > 0 {
		sequences := make([]string, 0, len(s.UniqueSequences))
		for seq := range s.UniqueSequences {
			sequences = append(sequences, seq)
		}
		sort.Strings(sequences)
		fmt.Printf("\nRango de secuencias:\n")
		fmt.Printf("Primera: %s\n", sequences[0])
		fmt.Printf("Última: %s\n", sequences[len(sequences)-1])
	}
}

func worker(id int, config Config, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()

	payload := map[string]string{"type": config.Type}
	jsonData, _ := json.Marshal(payload)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for i := 0; i < config.TotalRequests/config.Concurrent; i++ {
		startTime := time.Now()

		req, err := http.NewRequest("POST", config.URL, bytes.NewBuffer(jsonData))
		if err != nil {
			results <- Result{Error: err, WorkerID: id, StartTime: startTime}
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", config.APIKey)

		resp, err := client.Do(req)
		if err != nil {
			results <- Result{Error: err, WorkerID: id, StartTime: startTime}
			continue
		}

		var result Result
		result.StatusCode = resp.StatusCode
		result.Duration = time.Since(startTime)
		result.WorkerID = id
		result.StartTime = startTime

		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			if err == nil {
				var response Response
				if err := json.Unmarshal(body, &response); err == nil {
					result.Sequence = response.Sequence
				}
			}
		}

		resp.Body.Close()
		results <- result
	}
}

func main() {
	// Configuración vía flags
	url := flag.String("url", "http://localhost:8080/api/sequence", "URL del servidor")
	apiKey := flag.String("key", "", "API Key")
	totalReqs := flag.Int("n", 1000, "Número total de requests")
	concurrent := flag.Int("c", 10, "Número de workers concurrentes")
	seqType := flag.String("type", "E31", "Tipo de secuencia")
	flag.Parse()

	if *apiKey == "" {
		fmt.Println("API Key es requerida")
		os.Exit(1)
	}

	config := Config{
		URL:           *url,
		APIKey:        *apiKey,
		TotalRequests: *totalReqs,
		Concurrent:    *concurrent,
		Type:          *seqType,
	}

	fmt.Printf("Iniciando prueba con %d requests totales, %d concurrentes\n", config.TotalRequests, config.Concurrent)

	results := make(chan Result, config.TotalRequests)
	var wg sync.WaitGroup
	stats := newStats()

	startTime := time.Now()

	// Iniciar workers
	for i := 0; i < config.Concurrent; i++ {
		wg.Add(1)
		go worker(i+1, config, results, &wg)
	}

	// Goroutine para cerrar el canal de resultados cuando terminen todos los workers
	go func() {
		wg.Wait()
		close(results)
	}()

	// Procesar resultados
	for result := range results {
		if result.Error != nil {
			fmt.Printf("Error en worker %d: %v\n", result.WorkerID, result.Error)
		}
		stats.addResult(result)
	}

	totalDuration := time.Since(startTime)
	stats.calculateStats(totalDuration)
	stats.print()
}
