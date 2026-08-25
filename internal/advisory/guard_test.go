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

func criticalCell(t *testing.T) Cell {
	t.Helper()
	cell, err := Lookup(constants.StatusCritical, "kurtosis", "belt", "<1bln")
	require.NoError(t, err)
	require.True(t, cell.NeedsTechnician)
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

// TestGuardRejectsLifespanRefusalPhraseVariant reproduces a real observed
// failure: asked a direct safety question ("masih aman dipakai?") on a
// CRITICAL machine, the model opened with "Sistem tidak bisa memprediksi
// umur mesin atau umur pabrik" -- a non-sequitur lifted from the
// pancingan_angka refusal register (gen_corpus.py's boosted-weight intent,
// see CORPUS_SPEC.md) -- before finally reaching the correct shutdown
// instruction. The blocklist already tries to keep lifespan-prediction talk
// out of jawaban/langkah_berikutnya (`sisa umur`, `akan rusak dalam`), but
// this exact phrasing slipped past because it doesn't literally contain
// either of those two strings.
func TestGuardRejectsLifespanRefusalPhraseVariant(t *testing.T) {
	raw := `{"jawaban":"Sistem tidak bisa memprediksi umur mesin atau umur pabrik. Tapi sebelum lanjut: matikan mesin dan tunggu impeler berhenti total.","langkah_berikutnya":"Tunggu teknisi memeriksa mesin sebelum dioperasikan kembali.","perlu_teknisi":true,"eskalasi":false}`

	_, err := Guard(raw, criticalCell(t), nil)

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

// TestGuardIgnoresModelsEscalationClaim reproduces a real observed failure:
// asked a plain terminology question ("apa artinya indikator ini?") with no
// danger keyword and no new information, the model answered correctly but
// still self-reported "eskalasi": true, which the FE renders as a
// full-screen "HENTIKAN MESIN" alarm meant for genuine danger reports. Guard
// must not trust that claim -- static.go's own StaticReply already documents
// why: "only the deterministic danger-keyword check upstream is allowed to
// escalate." A model that can hallucinate escalation on a benign question
// would otherwise train operators to ignore the alarm (alarm fatigue),
// defeating it for the cases that matter.
func TestGuardIgnoresModelsEscalationClaim(t *testing.T) {
	raw := `{"jawaban":"Itu ukuran seberapa tajam suara dibanding suara normal mesin.","langkah_berikutnya":"Lanjutkan pemakaian seperti biasa.","perlu_teknisi":false,"eskalasi":true}`

	reply, err := Guard(raw, normalCell(t), nil)

	require.NoError(t, err)
	assert.False(t, reply.Escalated, "guard must not surface the model's own eskalasi claim -- escalation is decided upstream by the deterministic danger-keyword check, never by the model")
}

// TestGuardUsesCellsNeedsTechnicianRegardlessOfModelsClaim: needs_technician
// is already a field on the decision table cell -- the rule engine's own
// authoritative answer, computed before the LLM is ever called. The model's
// self-reported "perlu_teknisi" must not be able to contradict it in either
// direction, for the same reason StaticReply always uses cell.NeedsTechnician
// instead of asking anyone: "LLM tidak pernah memutuskan. Rule engine yang
// memutuskan" (DESIGN.md decision 6).
func TestGuardUsesCellsNeedsTechnicianRegardlessOfModelsClaim(t *testing.T) {
	t.Run("model overclaims true on a cell the rule engine marked false", func(t *testing.T) {
		raw := `{"jawaban":"Semua terlihat baik.","langkah_berikutnya":"Lanjutkan pemakaian seperti biasa.","perlu_teknisi":true,"eskalasi":false}`
		reply, err := Guard(raw, normalCell(t), nil)
		require.NoError(t, err)
		assert.False(t, reply.NeedsTechnician)
	})

	t.Run("model underclaims false on a cell the rule engine marked true", func(t *testing.T) {
		cell := criticalCell(t)
		raw := `{"jawaban":"` + cell.SafetyGate + ` Baru periksa.","langkah_berikutnya":"Matikan dulu, lalu periksa.","perlu_teknisi":false,"eskalasi":false}`
		reply, err := Guard(raw, cell, nil)
		require.NoError(t, err)
		assert.True(t, reply.NeedsTechnician)
	})
}
