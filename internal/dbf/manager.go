// internal/dbf/manager.go
package dbf

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LindsayBradford/go-dbf/godbf"
)

// ComprobanteTipo representa la estructura de un tipo de comprobante
type ComprobanteTipo struct {
	NCFTipo    string `json:"tipo"`
	CodPFF     int    `json:"cod_pf_f"`
	Nombre     string `json:"nombre"`
	Resumen    string `json:"resumen"`
	Numero     string `json:"numero"`
	Numero1    int64  `json:"secuencia_actual"`
	Numero2    int64  `json:"secuencia_hasta"`
	FechaDoc   string `json:"fecha_vencimiento"`
	Minimo     int64  `json:"minimo"`
	CantSecuen int64  `json:"cantidad_secuencias"`
}

// Manager maneja las operaciones con archivos DBF (y opcionalmente CDX)
type Manager struct {
	mu      sync.Mutex
	dbfPath string
	cdxPath string
	logFile *os.File
}

// NewManager crea una nueva instancia de Manager
func NewManager(dbfPath string) (*Manager, error) {
	logFile, err := os.OpenFile("sequence.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("error creating log file: %v", err)
	}

	// Como el CDX no lo manejamos con la librería godbf, podrías simplemente
	// omitir esta parte o validarla como en tu ejemplo original.
	cdxPath := strings.TrimSuffix(dbfPath, ".DBF") + ".CDX"
	if _, err := os.Stat(cdxPath); os.IsNotExist(err) {
		log.Printf("Warning: CDX file not found: %s (continúa sin índice)", cdxPath)
		// Podrías retornar un error o simplemente seguir.
	}

	return &Manager{
		dbfPath: dbfPath,
		cdxPath: cdxPath,
		logFile: logFile,
	}, nil
}

// Log graba en el log local y en el archivo 'sequence.log'
func (m *Manager) Log(message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fullMsg := fmt.Sprintf("%s: %s\n", timestamp, message)

	log.Print(fullMsg)
	_, _ = m.logFile.WriteString(fullMsg)
}

// parseInt ayuda a convertir cadenas a int64 (retorna 0 en caso de error)
func parseInt(s string) int64 {
	val, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return val
}

// GetRecordTypes obtiene todos los tipos de comprobantes usando la librería go-dbf
func (m *Manager) GetRecordTypes() ([]ComprobanteTipo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Abrimos el archivo DBF con godbf
	table, err := godbf.NewFromFile(m.dbfPath, "latin1") // Ajusta la codificación si es necesario
	if err != nil {
		return nil, fmt.Errorf("error abriendo DBF: %v", err)
	}

	var tipos []ComprobanteTipo

	// 2. Iteramos sobre los registros
	totalRecords := table.NumberOfRecords()
	for i := 0; i < totalRecords; i++ {
		// Si está marcado como eliminado, lo ignoramos
		if table.RowIsDeleted(i) {
			continue
		}

		// Lee los valores de cada campo
		codPffStr, _ := table.FieldValueByName(i, "COD_PF_F")
		nombreStr, _ := table.FieldValueByName(i, "NOMBRE")
		resumenStr, _ := table.FieldValueByName(i, "RESUMEN")
		numeroStr, _ := table.FieldValueByName(i, "NUMERO")
		num1Str, _ := table.FieldValueByName(i, "NUMERO_1")
		num2Str, _ := table.FieldValueByName(i, "NUMERO_2")
		fecDocStr, _ := table.FieldValueByName(i, "FEC_DOC")
		minStr, _ := table.FieldValueByName(i, "MINIMO")
		cantsStr, _ := table.FieldValueByName(i, "CANTSECUEN")

		// Convertir a numérico donde corresponda
		codPffVal := int(parseInt(codPffStr))
		num1Val := parseInt(num1Str)
		num2Val := parseInt(num2Str)
		minVal := parseInt(minStr)
		cantsVal := parseInt(cantsStr)

		// Creamos el struct
		comprob := ComprobanteTipo{
			CodPFF:     codPffVal,
			Nombre:     strings.TrimSpace(nombreStr),
			Resumen:    strings.TrimSpace(resumenStr),
			Numero:     strings.TrimSpace(numeroStr),
			Numero1:    num1Val,
			Numero2:    num2Val,
			FechaDoc:   strings.TrimSpace(fecDocStr),
			Minimo:     minVal,
			CantSecuen: cantsVal,
		}

		// Extraer el NCF si "Numero" tiene al menos 3 caracteres
		if len(comprob.Numero) >= 3 {
			comprob.NCFTipo = comprob.Numero[:3]
		}

		tipos = append(tipos, comprob)
	}

	return tipos, nil
}

// GetSequence busca el registro cuyo NUMERO comience con 'tipo',
// incrementa NUMERO_1 en 1, guarda el DBF y devuelve la nueva secuencia
func (m *Manager) GetSequence(tipo string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Abrimos el DBF
	table, err := godbf.NewFromFile(m.dbfPath, "latin1")
	if err != nil {
		return "", fmt.Errorf("error abriendo DBF: %v", err)
	}

	var found bool
	var newSeqVal int64

	// 2. Buscamos el registro que tenga NUMERO con el prefijo buscado
	totalRecords := table.NumberOfRecords()
	for i := 0; i < totalRecords; i++ {
		if table.RowIsDeleted(i) {
			continue
		}

		numeroStr, err := table.FieldValueByName(i, "NUMERO")
		if err != nil {
			continue // O maneja el error como prefieras
		}
		numeroStr = strings.TrimSpace(numeroStr)

		// ¿Es el que buscamos?
		if strings.HasPrefix(numeroStr, tipo) {
			// 2.1 Leer el campo NUMERO_1
			seqVal, _ := table.Int64FieldValueByName(i, "NUMERO_1")

			// 2.2 Incrementamos
			newSeqVal = seqVal + 1

			// 2.3 Guardamos de vuelta en la tabla (como string)
			err := table.SetFieldValueByName(i, "NUMERO_1", strconv.FormatInt(newSeqVal, 10))
			if err != nil {
				return "", fmt.Errorf("error setFieldValue: %v", err)
			}

			found = true
			break
		}
	}

	if !found {
		return "", fmt.Errorf("tipo de comprobante no encontrado: %s", tipo)
	}

	// 3. Guardamos cambios al archivo DBF
	err = godbf.SaveToFile(table, m.dbfPath)
	if err != nil {
		return "", fmt.Errorf("error guardando DBF: %v", err)
	}

	// 4. Construimos la secuencia final y hacemos log
	//    Por ejemplo: B03 + zero-padded de 10 dígitos => "B030000000123"
	sequence := fmt.Sprintf("%s%010d", tipo, newSeqVal)
	m.Log(fmt.Sprintf("Generated sequence: %s", sequence))

	return sequence, nil
}
