package dbf_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"ecf-sequence-server/internal/dbf"

	"github.com/LindsayBradford/go-dbf/godbf"
)

// testData representa la estructura de los datos de prueba
type testData struct {
	Tipo            string `json:"tipo"`
	CodPFF          int    `json:"cod_pf_f"`
	Nombre          string `json:"nombre"`
	Resumen         string `json:"resumen"`
	Numero          string `json:"numero"`
	SecuenciaActual int64  `json:"secuencia_actual"`
	SecuenciaHasta  int64  `json:"secuencia_hasta"`
	FechaVenc       string `json:"fecha_vencimiento"`
	Minimo          int64  `json:"minimo"`
	CantSecuencias  int64  `json:"cantidad_secuencias"`
}

// createTestDBF crea un archivo DBF de prueba con datos reales
func createTestDBF(t *testing.T, filename string) string {
	t.Helper()

	// Asegurarse que el directorio existe
	dir := path.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("No se pudo crear el directorio: %v", err)
	}

	// Crear un nuevo archivo DBF con codificación latin1
	db := godbf.New("latin1")

	// Añadir campos usando los métodos específicos
	if err := db.AddNumberField("COD_PF_F", 3, 0); err != nil {
		t.Fatalf("Error añadiendo campo COD_PF_F: %v", err)
	}
	if err := db.AddTextField("NOMBRE", 50); err != nil {
		t.Fatalf("Error añadiendo campo NOMBRE: %v", err)
	}
	if err := db.AddTextField("RESUMEN", 50); err != nil {
		t.Fatalf("Error añadiendo campo RESUMEN: %v", err)
	}
	if err := db.AddTextField("NUMERO", 20); err != nil {
		t.Fatalf("Error añadiendo campo NUMERO: %v", err)
	}
	if err := db.AddNumberField("NUMERO_1", 10, 0); err != nil {
		t.Fatalf("Error añadiendo campo NUMERO_1: %v", err)
	}
	if err := db.AddNumberField("NUMERO_2", 10, 0); err != nil {
		t.Fatalf("Error añadiendo campo NUMERO_2: %v", err)
	}
	if err := db.AddDateField("FEC_DOC"); err != nil {
		t.Fatalf("Error añadiendo campo FEC_DOC: %v", err)
	}
	if err := db.AddNumberField("MINIMO", 10, 0); err != nil {
		t.Fatalf("Error añadiendo campo MINIMO: %v", err)
	}
	if err := db.AddNumberField("CANTSECUEN", 10, 0); err != nil {
		t.Fatalf("Error añadiendo campo CANTSECUEN: %v", err)
	}

	// Datos de ejemplo (los mismos que proporcionaste)
	testDataJSON := `[{"tipo":"E32","cod_pf_f":1,"nombre":"CONSUMIDOR FINAL","resumen":"","numero":"E3200","secuencia_actual":0,"secuencia_hasta":0,"fecha_vencimiento":"20261231","minimo":100,"cantidad_secuencias":1000000},{"tipo":"B03","cod_pf_f":6,"nombre":"NOTA DE DEBITOS","resumen":"","numero":"B03","secuencia_actual":0,"secuencia_hasta":2,"fecha_vencimiento":"","minimo":0,"cantidad_secuencias":0}]`

	var records []testData
	if err := json.Unmarshal([]byte(testDataJSON), &records); err != nil {
		t.Fatalf("Error parseando JSON de prueba: %v", err)
	}

	// Añadir registros al DBF
	for _, record := range records {
		recordNum, err := db.AddNewRecord()
		if err != nil {
			t.Fatalf("Error creando nuevo registro: %v", err)
		}

		// Establecer valores usando SetFieldValueByName
		fields := map[string]string{
			"COD_PF_F":   stringify(record.CodPFF),
			"NOMBRE":     record.Nombre,
			"RESUMEN":    record.Resumen,
			"NUMERO":     record.Numero,
			"NUMERO_1":   stringify(record.SecuenciaActual),
			"NUMERO_2":   stringify(record.SecuenciaHasta),
			"FEC_DOC":    record.FechaVenc,
			"MINIMO":     stringify(record.Minimo),
			"CANTSECUEN": stringify(record.CantSecuencias),
		}

		for fieldName, value := range fields {
			if err := db.SetFieldValueByName(recordNum, fieldName, value); err != nil {
				t.Fatalf("Error estableciendo valor para %s: %v", fieldName, err)
			}
		}
	}

	// Guardar el archivo DBF
	if err := godbf.SaveToFile(db, filename); err != nil {
		t.Fatalf("Error guardando archivo DBF: %v", err)
	}

	return filename
}

func stringify(v interface{}) string {
	return fmt.Sprintf("%v", v)
}

func TestNewManager(t *testing.T) {
	currentDir, _ := os.Getwd()
	realDBFPath := path.Join(currentDir, "FAC_PF_M.DBF") // Usar ruta absoluta
	fmt.Println("Real DBF path:", realDBFPath)

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
			name:    "path vacío",
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

func TestManager_GetRecordTypes(t *testing.T) {
	mgr, err := dbf.NewManager(path.Join("FAC_PF_M.DBF"))
	if err != nil {
		t.Fatalf("Error creando Manager: %v", err)
	}

	tipos, err := mgr.GetRecordTypes()
	if err != nil {
		t.Errorf("GetRecordTypes() error = %v", err)
		return
	}

	// Verificar que tenemos los tipos esperados
	expectedTypes := map[string]bool{
		"E32": true,
		"E31": true,
		"B03": true,
		"E44": true,
		"E34": true,
		"B11": true,
		"B12": true,
		"B13": true,
		"E45": true,
		"E35": false,
	}

	for _, tipo := range tipos {
		if !expectedTypes[tipo.NCFTipo] {
			t.Errorf("GetRecordTypes() tipo inesperado = %v", tipo.NCFTipo)
		}
	}
}

func TestManager_GetSequence(t *testing.T) {
	// Usar el archivo real
	mgr, err := dbf.NewManager(path.Join("FAC_PF_M.DBF"))
	if err != nil {
		t.Fatalf("Error creando Manager: %v", err)
	}

	tests := []struct {
		name     string
		tipo     string
		wantErr  bool
		validate func(string) bool
	}{
		{
			name:    "tipo válido B03",
			tipo:    "B03",
			wantErr: false,
			validate: func(seq string) bool {
				return len(seq) == 13 && seq[:3] == "B03"
			},
		},
		{
			name:    "tipo válido E31",
			tipo:    "E31",
			wantErr: false,
			validate: func(seq string) bool {
				return len(seq) == 13 && seq[:3] == "E31"
			},
		},
		{
			name:    "tipo inexistente",
			tipo:    "XXX",
			wantErr: true,
			validate: func(seq string) bool {
				return true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seq, err := mgr.GetSequence(tt.tipo)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSequence() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !tt.validate(seq) {
				t.Errorf("GetSequence() = %v, formato inválido", seq)
			}
		})
	}
}

func TestConcurrency(t *testing.T) {
	mgr, err := dbf.NewManager(path.Join("FAC_PF_M.DBF"))
	if err != nil {
		t.Fatalf("Error creando Manager: %v", err)
	}

	const numGoroutines = 5
	results := make(chan string, numGoroutines)
	errors := make(chan error, numGoroutines)

	// Ejecutar múltiples goroutines concurrentemente
	for i := 0; i < numGoroutines; i++ {
		go func() {
			seq, err := mgr.GetSequence("E31")
			if err != nil {
				errors <- err
				results <- ""
				return
			}
			results <- seq
		}()
	}

	// Recolectar resultados
	seqs := make(map[string]bool)
	for i := 0; i < numGoroutines; i++ {
		select {
		case err := <-errors:
			t.Errorf("Error en goroutine: %v", err)
		case seq := <-results:
			if seq != "" {
				if seqs[seq] {
					t.Errorf("Secuencia duplicada detectada: %s", seq)
				}
				seqs[seq] = true
			}
		case <-time.After(5 * time.Second):
			t.Error("Timeout esperando resultados")
		}
	}
}
