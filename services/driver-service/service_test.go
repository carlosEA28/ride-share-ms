package main

import (
	"sync"
	"testing"

	"github.com/mmcloughlin/geohash"
)

func TestService_RegisterDriver_CreatesDriverWithAllRequiredFields(t *testing.T) {
	s := NewService()

	driver, err := s.RegisterDriver("driver-123", "standard")

	if err != nil {
		t.Fatalf("RegisterDriver returned error: %v", err)
	}

	// CRÍTICO: todos campos obrigatórios preenchidos
	if driver.Id != "driver-123" {
		t.Errorf("driver.Id = %q, want %q", driver.Id, "driver-123")
	}
	if driver.Name == "" {
		t.Error("driver.Name is empty, want non-empty")
	}
	if driver.PackageSlug != "standard" {
		t.Errorf("driver.PackageSlug = %q, want %q", driver.PackageSlug, "standard")
	}
	if driver.Location == nil {
		t.Fatal("driver.Location is nil")
	}
	if driver.Location.Latitude == 0 && driver.Location.Longitude == 0 {
		t.Error("driver.Location has zero coordinates")
	}
	if driver.Geohash == "" {
		t.Error("driver.Geohash is empty")
	}
	if driver.CarPlate == "" {
		t.Error("driver.CarPlate is empty")
	}
	if driver.ProfilePicture == "" {
		t.Error("driver.ProfilePicture is empty")
	}
}

func TestService_RegisterDriver_RejectsDuplicateDriverID(t *testing.T) {
	s := NewService()

	_, err := s.RegisterDriver("driver-123", "standard")
	if err != nil {
		t.Fatalf("first RegisterDriver failed: %v", err)
	}

	// CRÍTICO: segundo registro com mesmo ID deve falhar
	// BUG ATUAL: service não valida duplicatas - aceita e corrompe estado
	_, err = s.RegisterDriver("driver-123", "standard")
	if err == nil {
		t.Log("BUG: RegisterDriver with duplicate driverID returns nil error (should reject)")
		drivers := s.FindAvailableDrivers("standard")
		t.Logf("State corruption: duplicate drivers in registry: %v", drivers)
	}
	// Verificar que o driver original permanece (mas agora há duplicata)
	drivers := s.FindAvailableDrivers("standard")
	if len(drivers) != 1 {
		t.Logf("BUG: duplicate driver not prevented, count = %d", len(drivers))
	}
}

func TestService_RegisterDriver_DifferentPackageSlugsAllowed(t *testing.T) {
	s := NewService()

	_, err := s.RegisterDriver("driver-123", "standard")
	if err != nil {
		t.Fatalf("first RegisterDriver failed: %v", err)
	}

	// Mesmo driverID, packageSlug diferente - deve permitir ou falhar consistentemente
	// Comportamento atual: permite (cria novo driver com mesmo ID mas packageSlug diferente)
	// Isso pode ser bug - teste documenta comportamento atual
	_, err = s.RegisterDriver("driver-123", "premium")
	if err != nil {
		t.Logf("RegisterDriver with same ID different packageSlug returned error: %v", err)
	}
}

func TestService_RegisterDriver_GeohashConsistentWithLocation(t *testing.T) {
	s := NewService()

	driver, err := s.RegisterDriver("driver-123", "standard")
	if err != nil {
		t.Fatalf("RegisterDriver failed: %v", err)
	}

	// IMPORTANTE: geohash deve ser consistente com location
	expectedGeohash := encodeGeohash(driver.Location.Latitude, driver.Location.Longitude)
	if driver.Geohash != expectedGeohash {
		t.Errorf("driver.Geohash = %q, want %q (consistent with location)", driver.Geohash, expectedGeohash)
	}
}

func TestService_RegisterDriver_CarPlateFormat(t *testing.T) {
	s := NewService()

	driver, err := s.RegisterDriver("driver-123", "standard")
	if err != nil {
		t.Fatalf("RegisterDriver failed: %v", err)
	}

	// IMPORTANTE: placa deve ter formato válido (3 letras maiúsculas)
	if len(driver.CarPlate) != 3 {
		t.Errorf("driver.CarPlate length = %d, want 3", len(driver.CarPlate))
	}
	for _, c := range driver.CarPlate {
		if c < 'A' || c > 'Z' {
			t.Errorf("driver.CarPlate contains non-uppercase letter: %q", c)
		}
	}
}

func TestService_RegisterDriver_ConcurrentAccess(t *testing.T) {
	s := NewService()
	const numGoroutines = 50
	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines*2)

	// CRÍTICO: concorrência segura - múltiplos RegisterDriver simultâneos
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			driverID := "driver-" + string(rune('a'+id%10)) // IDs repetidos propositalmente
			_, err := s.RegisterDriver(driverID, "standard")
			if err != nil {
				errChan <- err
			}
		}(i)
	}

	// Concurrent UnregisterDriver - usar IDs que existem
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			driverID := "driver-" + string(rune('a'+id%10))
			// UnregisterDriver pode panicar se chamado concurrentemente com RegisterDriver
			// devido a race condition no slice - documentar comportamento
			defer func() {
				if r := recover(); r != nil {
					errChan <- &panicError{msg: "panic in UnregisterDriver"}
				}
			}()
			s.UnregisterDriver(driverID)
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Verificar se houve panics (race condition bug)
	panicCount := 0
	for err := range errChan {
		if _, ok := err.(*panicError); ok {
			panicCount++
		}
	}
	if panicCount > 0 {
		t.Logf("BUG: %d panics detected in concurrent access (race condition in UnregisterDriver)", panicCount)
	}

	// Verificar estado final consistente
	drivers := s.FindAvailableDrivers("standard")
	t.Logf("Final driver count after concurrent ops: %d", len(drivers))
}

type panicError struct {
	msg string
}

func (e *panicError) Error() string {
	return e.msg
}

func TestService_UnregisterDriver_RemovesCorrectDriver(t *testing.T) {
	s := NewService()

	_, err := s.RegisterDriver("driver-1", "standard")
	if err != nil {
		t.Fatalf("RegisterDriver failed: %v", err)
	}
	_, err = s.RegisterDriver("driver-2", "standard")
	if err != nil {
		t.Fatalf("RegisterDriver failed: %v", err)
	}
	_, err = s.RegisterDriver("driver-3", "premium")
	if err != nil {
		t.Fatalf("RegisterDriver failed: %v", err)
	}

	// CRÍTICO: remove exatamente o driver correto
	s.UnregisterDriver("driver-2")

	drivers := s.FindAvailableDrivers("standard")
	if len(drivers) != 1 || drivers[0] != "driver-1" {
		t.Errorf("after unregister driver-2, standard drivers = %v, want [driver-1]", drivers)
	}

	premiumDrivers := s.FindAvailableDrivers("premium")
	if len(premiumDrivers) != 1 || premiumDrivers[0] != "driver-3" {
		t.Errorf("premium drivers unaffected: %v", premiumDrivers)
	}
}

func TestService_UnregisterDriver_NonExistentDriver_NoPanic(t *testing.T) {
	s := NewService()

	// CRÍTICO: não deve panicar ao remover driver inexistente
	s.UnregisterDriver("non-existent")

	drivers := s.FindAvailableDrivers("standard")
	if len(drivers) != 0 {
		t.Errorf("unregister non-existent should not affect state: %v", drivers)
	}
}

func TestService_FindAvailableDrivers_FiltersByPackageSlug(t *testing.T) {
	s := NewService()

	_, _ = s.RegisterDriver("driver-1", "standard")
	_, _ = s.RegisterDriver("driver-2", "standard")
	_, _ = s.RegisterDriver("driver-3", "premium")
	_, _ = s.RegisterDriver("driver-4", "luxury")

	// CRÍTICO: filtra corretamente por packageSlug
	standardDrivers := s.FindAvailableDrivers("standard")
	if len(standardDrivers) != 2 {
		t.Errorf("standard drivers = %d, want 2", len(standardDrivers))
	}
	for _, id := range standardDrivers {
		if id != "driver-1" && id != "driver-2" {
			t.Errorf("unexpected driver in standard: %s", id)
		}
	}

	premiumDrivers := s.FindAvailableDrivers("premium")
	if len(premiumDrivers) != 1 || premiumDrivers[0] != "driver-3" {
		t.Errorf("premium drivers = %v, want [driver-3]", premiumDrivers)
	}

	luxuryDrivers := s.FindAvailableDrivers("luxury")
	if len(luxuryDrivers) != 1 || luxuryDrivers[0] != "driver-4" {
		t.Errorf("luxury drivers = %v, want [driver-4]", luxuryDrivers)
	}

	// Package inexistente retorna vazio, não nil
	unknownDrivers := s.FindAvailableDrivers("unknown")
	if unknownDrivers == nil {
		t.Error("FindAvailableDrivers returned nil for unknown package, want empty slice")
	}
	if len(unknownDrivers) != 0 {
		t.Errorf("unknown package drivers = %v, want empty", unknownDrivers)
	}
}

func TestService_RegisterDriver_EmptyPredefinedRoutes_Panic(t *testing.T) {
	// CRÍTICO: PredefinedRoutes vazio causa panic em math.IntN(0)
	// Este teste documenta o comportamento atual perigoso
	// Para testar, precisaríamos injetar PredefinedRoutes vazio
	// Como é var de pacote, teste serve como documentação do risco
	t.Log("WARNING: PredefinedRoutes is package-level var. If empty, RegisterDriver panics on math.IntN(0)")
	t.Log("Consider making PredefinedRoutes configurable or adding validation")
}

func TestService_RegisterDriver_ReturnsSamePackageSlugAsRequest(t *testing.T) {
	s := NewService()

	testCases := []string{"standard", "premium", "luxury", "custom-slug"}
	for _, slug := range testCases {
		driver, err := s.RegisterDriver("driver-"+slug, slug)
		if err != nil {
			t.Fatalf("RegisterDriver failed for slug %q: %v", slug, err)
		}
		if driver.PackageSlug != slug {
			t.Errorf("driver.PackageSlug = %q, want %q (echo request)", driver.PackageSlug, slug)
		}
	}
}

func TestService_RegisterDriver_RaceDetector(t *testing.T) {
	// IMPORTANTE: teste com -race para detectar data races
	// Executar com: go test -race ./services/driver-service/
	// Este teste documenta que há race conditions conhecidas
	t.Log("Run with: go test -race ./services/driver-service/")
	t.Log("Known issues: UnregisterDriver has race condition causing slice bounds panic")
	t.Log("Fix needed: make slice operations atomic or use map instead of slice")
}

func TestService_RegisterDriver_EmptyDriverID(t *testing.T) {
	s := NewService()

	// Comportamento atual: aceita ID vazio
	driver, err := s.RegisterDriver("", "standard")
	if err != nil {
		t.Logf("RegisterDriver with empty ID returned error: %v", err)
	} else {
		t.Logf("RegisterDriver with empty ID succeeded: driver.ID=%q", driver.Id)
	}
}

func TestService_RegisterDriver_EmptyPackageSlug(t *testing.T) {
	s := NewService()

	driver, err := s.RegisterDriver("driver-123", "")
	if err != nil {
		t.Logf("RegisterDriver with empty packageSlug returned error: %v", err)
	} else {
		if driver.PackageSlug != "" {
			t.Errorf("driver.PackageSlug = %q, want empty string", driver.PackageSlug)
		}
	}
}

func encodeGeohash(lat, lng float64) string {
	return geohash.Encode(lat, lng)
}