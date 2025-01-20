package dbf_test

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"sync"
	"testing"
	"time"

	"ecf-sequence-server/internal/dbf"
)

// TestNewManager prueba la creación de un Manager con distintos escenarios
func TestNewManager(t *testing.T) {
	// Ajusta la ruta según tu situación real:
	currentDir, _ := os.Getwd()
	realDBFPath := path.Join(currentDir, "FAC_PF_M.DBF")
	fmt.Println("Probando con DBF real:", realDBFPath)

	tests := []struct {
		name    string
		dbfPath string
		wantErr bool
	}{
		{
			name:    "archivo real existente",
			dbfPath: realDBFPath,
			wantErr: false,
		},
		{
			name:    "archivo inexistente",
			dbfPath: "no_existe.DBF",
			wantErr: true,
		},
		{
			name:    "ruta vacía",
			dbfPath: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := dbf.NewManager(tt.dbfPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && mgr == nil {
				t.Error("NewManager() retornó nil, se esperaba una instancia válida")
			}
		})
	}
}

// TestManager_GetRecordTypes prueba la obtención de tipos de comprobantes
func TestManager_GetRecordTypes(t *testing.T) {
	currentDir, _ := os.Getwd()
	realDBFPath := path.Join(currentDir, "FAC_PF_M.DBF")

	mgr, err := dbf.NewManager(realDBFPath)
	if err != nil {
		t.Fatalf("Error creando Manager: %v", err)
	}

	tipos, err := mgr.GetRecordTypes()
	if err != nil {
		t.Errorf("GetRecordTypes() error = %v", err)
		return
	}

	// Por ejemplo, podrías chequear si existen al menos algunos de los tipos que
	// usualmente están en tu DBF, como E31, E32, B03, etc.
	// Ajusta esta parte según tus datos reales:
	expected := map[string]bool{
		"B03": true,
		"E31": true,
		"E32": true,
		"E34": true,
	}

	found := 0
	for _, tipo := range tipos {
		if expected[tipo.NCFTipo] {
			found++
		}
	}
	if found == 0 {
		t.Errorf("No se encontraron tipos esperados (ej. B03, E31, E32). Revisa el contenido de tu DBF real.")
	}
}

// TestManager_GetSequence prueba la obtención de una secuencia para diferentes escenarios
func TestManager_GetSequence(t *testing.T) {
	currentDir, _ := os.Getwd()
	realDBFPath := path.Join(currentDir, "FAC_PF_M.DBF")

	mgr, err := dbf.NewManager(realDBFPath)
	if err != nil {
		t.Fatalf("Error creando Manager: %v", err)
	}

	// Ajusta los tipos y CTA que sepas que existen en tu DBF real
	tests := []struct {
		name      string
		tipo      string
		cta       string
		wantErr   bool
		wantStart string // Prefijo esperado de la secuencia
	}{
		{
			name:      "tipo existente (p.ej. E31), CTA A",
			tipo:      "E31",
			cta:       "A",
			wantErr:   false,
			wantStart: "E31",
		},
		{
			name:      "tipo existente (p.ej. B03), CTA A",
			tipo:      "B03",
			cta:       "A",
			wantErr:   false,
			wantStart: "B03",
		},
		{
			name:      "tipo existente (p.ej. B03), CTA B",
			tipo:      "B03",
			cta:       "B",
			wantErr:   false,
			wantStart: "B03",
		},
		{
			name:      "CTA inválida",
			tipo:      "B03",
			cta:       "X",
			wantErr:   true,
			wantStart: "",
		},
		{
			name:      "tipo inexistente",
			tipo:      "XXX",
			cta:       "A",
			wantErr:   true,
			wantStart: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seq, seqNum, err := mgr.GetSequence(tt.tipo, tt.cta)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSequence() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// Si esperamos error, no validamos el resto
			if tt.wantErr && err != nil {
				return
			}

			// Validar el prefijo de la secuencia, por ejemplo "B03"
			if tt.wantStart != "" {
				if len(seq) < len(tt.wantStart) {
					t.Errorf("Secuencia demasiado corta: %s", seq)
				} else {
					gotStart := seq[:len(tt.wantStart)]
					if gotStart != tt.wantStart {
						t.Errorf("GetSequence() = %v, el prefijo esperado era %v", seq, tt.wantStart)
					}
				}
			}

			// Validar que seqNum sea > 0 (si se incrementó con éxito)
			if seqNum <= 0 {
				t.Errorf("El número de secuencia devuelto debe ser > 0, se obtuvo %d", seqNum)
			}

			// Validar que la parte numérica en seq coincida con seqNum
			if tt.wantStart != "" {
				numPartStr := seq[len(tt.wantStart):]
				numPart, err := strconv.ParseInt(numPartStr, 10, 64)
				if err != nil {
					t.Errorf("No se pudo convertir a int la parte numérica de %s: %v", seq, err)
				} else if numPart != int64(seqNum) {
					t.Errorf("La parte numérica de la secuencia (%d) no coincide con seqNum (%d)", numPart, seqNum)
				}
			}
		})
	}
}

// TestConcurrency prueba el acceso concurrente a GetSequence
func TestConcurrency(t *testing.T) {
	currentDir, _ := os.Getwd()
	realDBFPath := path.Join(currentDir, "FAC_PF_M.DBF")

	mgr, err := dbf.NewManager(realDBFPath)
	if err != nil {
		t.Fatalf("Error creando Manager: %v", err)
	}

	const numGoroutines = 5
	results := make(chan string, numGoroutines)
	errors := make(chan error, numGoroutines)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Asume que E31 y CTA A existen en tu DBF. Ajusta si es necesario.
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			seq, _, err := mgr.GetSequence("E32", "A")
			if err != nil {
				errors <- err
				results <- ""
				return
			}
			results <- seq
		}()
	}

	// Cerramos los canales cuando terminen las goroutines
	go func() {
		wg.Wait()
		close(results)
		close(errors)
	}()

	seqs := make(map[string]bool)
	timeout := time.After(5 * time.Second)

	for {
		select {
		case err := <-errors:
			if err != nil {
				t.Errorf("Error en goroutine: %v", err)
			}
		case seq, ok := <-results:
			if !ok {
				results = nil
			} else {
				// Verificar duplicados en las secuencias generadas
				if seqs[seq] {
					t.Errorf("Secuencia duplicada detectada: %s", seq)
				}
				seqs[seq] = true
			}
		case <-timeout:
			t.Error("Timeout esperando resultados de goroutines")
			return
		}

		if results == nil && errors == nil {
			break
		} else if results == nil && len(errors) == 0 {
			// Si results está cerrado y no quedan errores en la cola
			break
		}
	}
}
