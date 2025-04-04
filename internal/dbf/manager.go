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

	//valida que el archivo DBF exista
	if _, err := os.Stat(dbfPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("el archivo DBF no existe: %s", dbfPath)
	}

	// Como el CDX no lo manejamos con la librería godbf, podrías simplemente
	// omitir esta parte o validarla como en tu ejemplo original.
	cdxPath := strings.TrimSuffix(dbfPath, ".DBF") + ".CDX"

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

// ...existing code...
func (m *Manager) GetSequence(tipo string, cta string) (string, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	table, err := godbf.NewFromFile(m.dbfPath, "latin1")
	if err != nil {
		return "", 0, fmt.Errorf("error abriendo DBF: %v", err)
	}

	var found bool
	var newSeqVal int64
	var fieldName string

	// Determina si se incrementa NUMERO_1 o NUMERO_2
	switch strings.ToUpper(cta) {
	case "A":
		fieldName = "NUMERO_1"
	case "B":
		fieldName = "NUMERO_2"
	default:
		return "", 0, fmt.Errorf("CTA no válido: %s", cta)
	}

	totalRecords := table.NumberOfRecords()
	for i := 0; i < totalRecords; i++ {
		if table.RowIsDeleted(i) {
			continue
		}
		numeroStr, err := table.FieldValueByName(i, "NUMERO")
		if err != nil {
			continue
		}
		numeroStr = strings.TrimSpace(numeroStr)

		if strings.HasPrefix(numeroStr, tipo) {
			seqVal, _ := table.Int64FieldValueByName(i, fieldName)
			newSeqVal = seqVal + 1

			err := table.SetFieldValueByName(i, fieldName, strconv.FormatInt(newSeqVal, 10))
			if err != nil {
				return "", 0, fmt.Errorf("error setFieldValue: %v", err)
			}
			found = true
			break
		}
	}

	if !found {
		return "", 0, fmt.Errorf("tipo de comprobante no encontrado: %s", tipo)
	}

	if err := godbf.SaveToFile(table, m.dbfPath); err != nil {
		return "", 0, fmt.Errorf("error guardando DBF: %v", err)
	}

	sequence := fmt.Sprintf("%s%010d", tipo, newSeqVal)
	m.Log(fmt.Sprintf("Generated sequence: %s", sequence))
	return sequence, newSeqVal, nil
}
