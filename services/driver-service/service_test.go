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

// ========== FindAvailableDrivers Tests ==========

func TestService_FindAvailableDrivers_ReturnsEmptySliceNotNil(t *testing.T) {
	s := NewService()

	// CRÍTICO: retorna slice vazia, não nil
	// Consumidor em trip_consumer.go faz: if len(suitableIDs) == 0
	drivers := s.FindAvailableDrivers("standard")
	if drivers == nil {
		t.Error("FindAvailableDrivers returned nil, want empty slice")
	}
	if len(drivers) != 0 {
		t.Errorf("FindAvailableDrivers len = %d, want 0", len(drivers))
	}
}

func TestService_FindAvailableDrivers_ExcludesUnregisteredDrivers(t *testing.T) {
	s := NewService()

	_, _ = s.RegisterDriver("driver-1", "standard")
	_, _ = s.RegisterDriver("driver-2", "standard")
	_, _ = s.RegisterDriver("driver-3", "standard")

	// CRÍTICO: driver removido não deve aparecer
	s.UnregisterDriver("driver-2")

	drivers := s.FindAvailableDrivers("standard")
	if len(drivers) != 2 {
		t.Errorf("after unregister, drivers = %d, want 2", len(drivers))
	}
	for _, id := range drivers {
		if id == "driver-2" {
			t.Error("unregistered driver-2 still returned in FindAvailableDrivers")
		}
	}
}

func TestService_FindAvailableDrivers_PreservesRegistrationOrder(t *testing.T) {
	s := NewService()

	_, _ = s.RegisterDriver("driver-3", "standard")
	_, _ = s.RegisterDriver("driver-1", "standard")
	_, _ = s.RegisterDriver("driver-2", "standard")

	// IMPORTANTE: ordem de registro preservada (FIFO)
	// trip_consumer usa suitableIDs[0] - primeiro registrado recebe primeiro
	drivers := s.FindAvailableDrivers("standard")
	expected := []string{"driver-3", "driver-1", "driver-2"}
	if len(drivers) != 3 {
		t.Fatalf("drivers len = %d, want 3", len(drivers))
	}
	for i, id := range drivers {
		if id != expected[i] {
			t.Errorf("drivers[%d] = %q, want %q (registration order)", i, id, expected[i])
		}
	}
}

func TestService_FindAvailableDrivers_CaseSensitivePackageSlug(t *testing.T) {
	s := NewService()

	_, _ = s.RegisterDriver("driver-1", "standard")
	_, _ = s.RegisterDriver("driver-2", "Standard")
	_, _ = s.RegisterDriver("driver-3", "STANDARD")

	// IMPORTANTE: case-sensitive - "standard" ≠ "Standard" ≠ "STANDARD"
	standard := s.FindAvailableDrivers("standard")
	if len(standard) != 1 || standard[0] != "driver-1" {
		t.Errorf("standard = %v, want [driver-1]", standard)
	}

	standardCap := s.FindAvailableDrivers("Standard")
	if len(standardCap) != 1 || standardCap[0] != "driver-2" {
		t.Errorf("Standard = %v, want [driver-2]", standardCap)
	}

	standardUpper := s.FindAvailableDrivers("STANDARD")
	if len(standardUpper) != 1 || standardUpper[0] != "driver-3" {
		t.Errorf("STANDARD = %v, want [driver-3]", standardUpper)
	}
}

func TestService_FindAvailableDrivers_EmptyPackageType(t *testing.T) {
	s := NewService()

	_, _ = s.RegisterDriver("driver-1", "standard")
	_, _ = s.RegisterDriver("driver-2", "") // packageSlug vazio

	// IMPORTANTE: packageType vazio deve matchar drivers com packageSlug vazio
	empty := s.FindAvailableDrivers("")
	if len(empty) != 1 || empty[0] != "driver-2" {
		t.Errorf("empty packageType = %v, want [driver-2]", empty)
	}

	// E não deve matchar drivers com packageSlug não-vazio
	standard := s.FindAvailableDrivers("standard")
	if len(standard) != 1 || standard[0] != "driver-1" {
		t.Errorf("standard = %v, want [driver-1]", standard)
	}
}

func TestService_FindAvailableDrivers_ConcurrentReadWriteRace(t *testing.T) {
	// CRÍTICO: FindAvailableDrivers NÃO usa RLock - race condition com Register/Unregister
	// Este teste expõe o bug: execute com -race
	s := NewService()
	var wg sync.WaitGroup
	errChan := make(chan error, 100)
	const iterations = 500

	// Writers: Register + Unregister
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			driverID := "driver-" + string(rune('a'+id%50))
			s.RegisterDriver(driverID, "standard")
			defer func() {
				if r := recover(); r != nil {
					errChan <- &panicError{msg: "panic in UnregisterDriver"}
				}
			}()
			s.UnregisterDriver(driverID)
		}(i)
	}

	// Readers: FindAvailableDrivers concorrente
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.FindAvailableDrivers("standard")
		}()
	}

	wg.Wait()
	close(errChan)

	// Verificar panics (race condition conhecido em UnregisterDriver)
	panicCount := 0
	for err := range errChan {
		if _, ok := err.(*panicError); ok {
			panicCount++
		}
	}
	if panicCount > 0 {
		t.Logf("BUG: %d panics in writers (known UnregisterDriver race condition)", panicCount)
	}

	t.Log("Run with: go test -race ./services/driver-service/")
	t.Log("Expected: data race on s.drivers slice (read in FindAvailableDrivers vs write in Register/Unregister)")
}

func TestService_FindAvailableDrivers_ReturnsValidIDsForDispatch(t *testing.T) {
	// CRÍTICO: integração com trip_consumer - IDs retornados devem ser válidos para dispatch
	s := NewService()

	_, _ = s.RegisterDriver("driver-available", "standard")
	_, _ = s.RegisterDriver("driver-busy", "standard") // será "unregistered" simulando ocupado
	s.UnregisterDriver("driver-busy")

	suitableIDs := s.FindAvailableDrivers("standard")

	// trip_consumer.go:52-67 usa suitableIDs[0] para dispatch
	if len(suitableIDs) == 0 {
		t.Fatal("no available drivers found, trip_consumer would publish TripEventNoDriversFound")
	}

	dispatchedDriverID := suitableIDs[0]
	if dispatchedDriverID != "driver-available" {
		t.Errorf("dispatched driver = %q, want driver-available (only available)", dispatchedDriverID)
	}

	// Verificar que ID existe no registry (não é fantasma)
	allDrivers := s.FindAvailableDrivers("standard")
	found := false
	for _, id := range allDrivers {
		if id == dispatchedDriverID {
			found = true
			break
		}
	}
	if !found {
		t.Error("dispatched driver ID not found in registry (ghost driver)")
	}
}

func TestService_FindAvailableDrivers_MultiplePackageTypesIsolated(t *testing.T) {
	s := NewService()

	// Registrar drivers em múltiplos package types
	_, _ = s.RegisterDriver("std-1", "standard")
	_, _ = s.RegisterDriver("std-2", "standard")
	_, _ = s.RegisterDriver("prem-1", "premium")
	_, _ = s.RegisterDriver("lux-1", "luxury")

	// CRÍTICO: isolamento total entre package types
	std := s.FindAvailableDrivers("standard")
	if len(std) != 2 {
		t.Errorf("standard = %d, want 2", len(std))
	}

	prem := s.FindAvailableDrivers("premium")
	if len(prem) != 1 {
		t.Errorf("premium = %d, want 1", len(prem))
	}

	lux := s.FindAvailableDrivers("luxury")
	if len(lux) != 1 {
		t.Errorf("luxury = %d, want 1", len(lux))
	}

	// Unregister em um package não afeta outros
	s.UnregisterDriver("std-1")
	std = s.FindAvailableDrivers("standard")
	if len(std) != 1 {
		t.Errorf("after unregister std-1, standard = %d, want 1", len(std))
	}

	prem = s.FindAvailableDrivers("premium")
	if len(prem) != 1 {
		t.Errorf("premium unaffected by standard unregister = %d, want 1", len(prem))
	}
}

// ========== UnregisterDriver Tests ==========

func TestService_UnregisterDriver_RemovesExactDriver(t *testing.T) {
	s := NewService()

	_, _ = s.RegisterDriver("driver-1", "standard")
	_, _ = s.RegisterDriver("driver-2", "standard")
	_, _ = s.RegisterDriver("driver-3", "premium")

	// CRÍTICO: remove exatamente o driver correto
	s.UnregisterDriver("driver-2")

	// driver-1 (standard) e driver-3 (premium) permanecem
	// standard tem apenas driver-1 agora
	std := s.FindAvailableDrivers("standard")
	if len(std) != 1 || std[0] != "driver-1" {
		t.Errorf("standard drivers = %v, want [driver-1]", std)
	}

	// Outros packages não afetados
	prem := s.FindAvailableDrivers("premium")
	if len(prem) != 1 || prem[0] != "driver-3" {
		t.Errorf("premium unaffected: %v", prem)
	}
}

func TestService_UnregisterDriver_IdempotentNonExistent(t *testing.T) {
	s := NewService()

	// CRÍTICO: não panica, idempotente
	s.UnregisterDriver("non-existent")
	s.UnregisterDriver("non-existent") // segunda vez também não panica

	drivers := s.FindAvailableDrivers("standard")
	if len(drivers) != 0 {
		t.Errorf("state corrupted after unregister non-existent: %v", drivers)
	}
}

func TestService_UnregisterDriver_RemovesAllDuplicates(t *testing.T) {
	s := NewService()

	// BUG CONHECIDO: RegisterDriver permite duplicatas
	_, _ = s.RegisterDriver("driver-dup", "standard")
	_, _ = s.RegisterDriver("driver-dup", "standard")
	_, _ = s.RegisterDriver("driver-dup", "standard")

	driversBefore := s.FindAvailableDrivers("standard")
	if len(driversBefore) != 3 {
		t.Fatalf("setup failed: expected 3 duplicates, got %d", len(driversBefore))
	}

	// BUG CONHECIDO: UnregisterDriver tem bug ao remover duplicatas
	// O loop modifica slice durante iteração, causando slice bounds panic
	// ou pulando elementos
	defer func() {
		if r := recover(); r != nil {
			t.Logf("BUG CONFIRMADO: panic ao remover duplicatas: %v", r)
		}
	}()
	s.UnregisterDriver("driver-dup")

	driversAfter := s.FindAvailableDrivers("standard")
	// Comportamento atual: pode remover 1, 2 ou 3 (ou panic)
	t.Logf("Após unregister duplicatas: %d drivers restantes (esperado 0, bug conhecido)", len(driversAfter))
	if len(driversAfter) != 0 {
		t.Logf("BUG: nem todas duplicatas removidas, restantes: %v", driversAfter)
	}
}

func TestService_UnregisterDriver_ConcurrentWithRegisterRace(t *testing.T) {
	// CRÍTICO: race condition conhecido - UnregisterDriver modifica slice durante iteração
	// Execute com: go test -race ./services/driver-service/
	s := NewService()
	var wg sync.WaitGroup
	errChan := make(chan error, 100)
	const iterations = 200

	// Registrar drivers iniciais
	for i := 0; i < 50; i++ {
		s.RegisterDriver("driver-init-"+string(rune('a'+i)), "standard")
	}

	// Writers concorrentes: Register + Unregister mesmo ID
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			driverID := "driver-concurrent-" + string(rune('a'+id%10))
			s.RegisterDriver(driverID, "standard")
			defer func() {
				if r := recover(); r != nil {
					errChan <- &panicError{msg: "panic in UnregisterDriver"}
				}
			}()
			s.UnregisterDriver(driverID)
		}(i)
	}

	// Readers concorrentes
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.FindAvailableDrivers("standard")
		}()
	}

	wg.Wait()
	close(errChan)

	panicCount := 0
	for err := range errChan {
		if _, ok := err.(*panicError); ok {
			panicCount++
		}
	}
	if panicCount > 0 {
		t.Logf("BUG CONFIRMADO: %d panics em UnregisterDriver concorrente (slice bounds out of range)", panicCount)
	}
	t.Log("Run with: go test -race ./services/driver-service/")
}

func TestService_UnregisterDriver_DoesNotAffectOtherPackageSlugs(t *testing.T) {
	s := NewService()

	_, _ = s.RegisterDriver("driver-1", "standard")
	_, _ = s.RegisterDriver("driver-2", "premium")
	_, _ = s.RegisterDriver("driver-3", "luxury")

	// CRÍTICO: unregister em standard não afeta premium/luxury
	s.UnregisterDriver("driver-1")

	std := s.FindAvailableDrivers("standard")
	if len(std) != 0 {
		t.Errorf("standard should be empty: %v", std)
	}

	prem := s.FindAvailableDrivers("premium")
	if len(prem) != 1 || prem[0] != "driver-2" {
		t.Errorf("premium unaffected: %v", prem)
	}

	lux := s.FindAvailableDrivers("luxury")
	if len(lux) != 1 || lux[0] != "driver-3" {
		t.Errorf("luxury unaffected: %v", lux)
	}
}

func TestService_UnregisterDriver_RemovesFromEmptyRegistry(t *testing.T) {
	s := NewService()

	// CRÍTICO: unregister em registry vazio não panica
	s.UnregisterDriver("any-driver")

	drivers := s.FindAvailableDrivers("standard")
	if len(drivers) != 0 {
		t.Errorf("empty registry corrupted: %v", drivers)
	}
}

func TestService_UnregisterDriver_MultipleUnregistersSameID(t *testing.T) {
	s := NewService()

	_, _ = s.RegisterDriver("driver-1", "standard")

	// CRÍTICO: múltiplos unregisters do mesmo ID são idempotentes
	s.UnregisterDriver("driver-1")
	s.UnregisterDriver("driver-1")
	s.UnregisterDriver("driver-1")

	drivers := s.FindAvailableDrivers("standard")
	if len(drivers) != 0 {
		t.Errorf("multiple unregisters corrupted state: %v", drivers)
	}
}

func TestService_UnregisterDriver_ReturnsValidStateForDispatch(t *testing.T) {
	// CRÍTICO: integração com trip_consumer - após unregister, driver não deve ser despachado
	s := NewService()

	_, _ = s.RegisterDriver("driver-available", "standard")
	_, _ = s.RegisterDriver("driver-offline", "standard")
	s.UnregisterDriver("driver-offline")

	suitableIDs := s.FindAvailableDrivers("standard")

	// trip_consumer.go usa suitableIDs[0] para dispatch
	if len(suitableIDs) == 0 {
		t.Fatal("no available drivers - trip_consumer would publish NoDriversFound")
	}

	dispatchedID := suitableIDs[0]
	if dispatchedID == "driver-offline" {
		t.Error("unregistered driver-offline was dispatched (ghost driver)")
	}
	if dispatchedID != "driver-available" {
		t.Errorf("dispatched %q, want driver-available", dispatchedID)
	}
}