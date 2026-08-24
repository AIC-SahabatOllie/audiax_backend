package advisory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"audiax/internal/constants"
)

func warningBeltCell(t *testing.T) Cell {
	t.Helper()
	cell, err := Lookup(constants.StatusWarning, "crest_factor", "belt", "1-6bln")
	require.NoError(t, err)
	require.NotEmpty(t, cell.SafetyGate)
	return cell
}

func normalCell(t *testing.T) Cell {
	t.Helper()
	cell, err := Lookup(constants.StatusNormal, "kurtosis", "belt", "<1bln")
	require.NoError(t, err)
	require.Empty(t, cell.SafetyGate)
	return cell
}

func TestGuardPassesWellFormedOutput(t *testing.T) {
	raw := `{"jawaban":"Statusnya WARNING, artinya perlu diperiksa dalam beberapa hari.","langkah_berikutnya":"Periksa ketegangan sabuk sesuai checklist.","perlu_teknisi":false,"eskalasi":false}`

	reply, err := Guard(raw, normalCell(t), nil)

	require.NoError(t, err)
	assert.Equal(t, "Statusnya WARNING, artinya perlu diperiksa dalam beberapa hari.", reply.Answer)
	assert.Equal(t, "Periksa ketegangan sabuk sesuai checklist.", reply.NextStep)
	assert.False(t, reply.NeedsTechnician)
	assert.False(t, reply.Escalated)
}

func TestGuardRejectsInvalidJSON(t *testing.T) {
	_, err := Guard(`not json`, normalCell(t), nil)
	assert.Error(t, err)
}

func TestGuardRejectsEmptyAnswer(t *testing.T) {
	raw := `{"jawaban":"","langkah_berikutnya":"Periksa baut.","perlu_teknisi":false,"eskalasi":false}`
	_, err := Guard(raw, normalCell(t), nil)
	assert.Error(t, err)
}

func TestGuardRejectsForbiddenDiagnosisPhrase(t *testing.T) {
	raw := `{"jawaban":"Ini kemungkinan bearing aus pada motor Anda.","langkah_berikutnya":"Ganti bearing.","perlu_teknisi":true,"eskalasi":false}`

	_, err := Guard(raw, normalCell(t), nil)

	assert.Error(t, err)
}

func TestGuardRejectsNumberOutsideWhitelist(t *testing.T) {
	raw := `{"jawaban":"Mesin ini kemungkinan akan bertahan 15 hari lagi.","langkah_berikutnya":"Tunggu saja.","perlu_teknisi":false,"eskalasi":false}`

	_, err := Guard(raw, normalCell(t), nil)

	assert.Error(t, err)
}

func TestGuardAllowsOrdinalsOneThroughNine(t *testing.T) {
	raw := `{"jawaban":"Ikuti langkah ke-3 dari checklist di atas.","langkah_berikutnya":"Kerjakan langkah 1 dulu.","perlu_teknisi":false,"eskalasi":false}`

	_, err := Guard(raw, normalCell(t), nil)

	assert.NoError(t, err)
}

func TestGuardAllowsNumbersFromHealthCardWhitelist(t *testing.T) {
	raw := `{"jawaban":"Nilai z sekarang 3.4, di atas ambang 3.0.","langkah_berikutnya":"Ikuti checklist.","perlu_teknisi":false,"eskalasi":false}`

	_, err := Guard(raw, normalCell(t), []float64{3.4, 3.0, 6.0})

	assert.NoError(t, err)
}

func TestGuardAllowsNumbersQuotedVerbatimFromCell(t *testing.T) {
	cell := warningBeltCell(t)
	raw := `{"jawaban":"Lendutan wajar sekitar 1 cm per 100 cm jarak puli, sesuai checklist.","langkah_berikutnya":"` + cell.SafetyGate + ` Lalu periksa sabuk.","perlu_teknisi":false,"eskalasi":false}`

	_, err := Guard(raw, cell, nil)

	assert.NoError(t, err)
}

func TestGuardRequiresSafetyGateOrderingWhenCellHasOne(t *testing.T) {
	cell := warningBeltCell(t)

	t.Run("passes when shutdown precedes inspection", func(t *testing.T) {
		raw := `{"jawaban":"` + cell.SafetyGate + ` Baru periksa ketegangan sabuknya.","langkah_berikutnya":"Matikan dulu, lalu periksa sabuk.","perlu_teknisi":false,"eskalasi":false}`
		_, err := Guard(raw, cell, nil)
		assert.NoError(t, err)
	})

	t.Run("fails when the shutdown instruction is missing", func(t *testing.T) {
		raw := `{"jawaban":"Coba periksa ketegangan sabuknya dulu.","langkah_berikutnya":"Periksa sabuk.","perlu_teknisi":false,"eskalasi":false}`
		_, err := Guard(raw, cell, nil)
		assert.Error(t, err)
	})

	t.Run("fails when inspection is instructed before shutdown", func(t *testing.T) {
		raw := `{"jawaban":"Periksa dulu ketegangan sabuknya, baru matikan mesin dan tunggu impeler berhenti.","langkah_berikutnya":"Periksa sabuk.","perlu_teknisi":false,"eskalasi":false}`
		_, err := Guard(raw, cell, nil)
		assert.Error(t, err)
	})
}

func TestGuardSkipsSafetyCheckWhenCellHasNoSafetyGate(t *testing.T) {
	raw := `{"jawaban":"Semua terlihat baik, tidak ada tindakan yang diperlukan.","langkah_berikutnya":"Lanjutkan pemakaian seperti biasa.","perlu_teknisi":false,"eskalasi":false}`

	_, err := Guard(raw, normalCell(t), nil)

	assert.NoError(t, err)
}
