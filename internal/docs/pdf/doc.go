// Package pdf converte PDF em blocos normalizados.
// A implementação com go-fitz fica atrás de build tag cgo.
package pdf

import "errors"

// ErrScanned indica um PDF sem camada de texto suficiente (provável
// digitalização). A v1 não faz OCR: o chamador deve marcar o documento como
// `needs_ocr`.
var ErrScanned = errors.New("pdf: sem camada de texto (provável PDF escaneado) — OCR não suportado na v1")
