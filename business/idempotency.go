package business

import (
	"crypto/rand"
	"encoding/hex"
)

// NewIdempotencyKey генерує свіжий UUID v4, придатний для заголовка
// Idempotency-Key, якого очікують [Client.PreparePayment] та
// [Client.CreateSalaryRegistry]. Ентропія беруться з crypto/rand —
// зовнішніх залежностей нема.
//
// Семантика: ключ — це ідентифікатор «спроби» операції. Якщо мережа
// впала і ти ретраїш виклик із тим самим ключем, банк поверне ту саму
// відповідь, не дублюючи операцію. Новий логічний платіж потребує
// нового ключа.
//
//	key := business.NewIdempotencyKey()
//	out, err := cli.PreparePayment(ctx, key, &business.PaymentRequest{...})
//
// Панікує, якщо crypto/rand повертає помилку — це означає голод по
// ентропії на рівні ОС і ні в чому невиправний стан.
func NewIdempotencyKey() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("business: crypto/rand failed: " + err.Error())
	}
	// Виставити версію (4) і variant (RFC 4122) біти за специфікацією
	// UUID v4 (RFC 4122 §4.4).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	// Формат 8-4-4-4-12 hex-цифр.
	var dst [36]byte
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])
	return string(dst[:])
}
