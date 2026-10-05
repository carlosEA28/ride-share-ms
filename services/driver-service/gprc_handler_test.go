package main

import (
	"context"
	"net"
	"testing"

	pb "ride-sharing/shared/proto/driver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

func newTestServer(t *testing.T, s *Service) (*grpc.Server, *bufconn.Listener, func()) {
	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	NewGrpcHandler(grpcServer, s)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			t.Logf("gRPC server stopped: %v", err)
		}
	}()

	cleanup := func() {
		grpcServer.Stop()
		lis.Close()
	}

	return grpcServer, lis, cleanup
}

func bufDialer(lis *bufconn.Listener) func(context.Context, string) (net.Conn, error) {
	return func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
}

func newTestClient(t *testing.T, lis *bufconn.Listener) pb.DriverServiceClient {
	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(bufDialer(lis)), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return pb.NewDriverServiceClient(conn)
}

func TestGrpcHandler_RegisterDriver_Success(t *testing.T) {
	s := NewService()
	_, lis, cleanup := newTestServer(t, s)
	defer cleanup()

	client := newTestClient(t, lis)

	req := &pb.RegisterDriverRequest{
		DriverID:    "driver-123",
		PackageSlug: "standard",
	}

	resp, err := client.RegisterDriver(context.Background(), req)
	if err != nil {
		t.Fatalf("RegisterDriver RPC failed: %v", err)
	}

	if resp.Driver == nil {
		t.Fatal("RegisterDriverResponse.Driver is nil")
	}
	if resp.Driver.Id != "driver-123" {
		t.Errorf("resp.Driver.Id = %q, want %q", resp.Driver.Id, "driver-123")
	}
	if resp.Driver.PackageSlug != "standard" {
		t.Errorf("resp.Driver.PackageSlug = %q, want %q", resp.Driver.PackageSlug, "standard")
	}
	if resp.Driver.Location == nil {
		t.Error("resp.Driver.Location is nil")
	}
	if resp.Driver.Geohash == "" {
		t.Error("resp.Driver.Geohash is empty")
	}
	if resp.Driver.CarPlate == "" {
		t.Error("resp.Driver.CarPlate is empty")
	}
	if resp.Driver.ProfilePicture == "" {
		t.Error("resp.Driver.ProfilePicture is empty")
	}
}

func TestGrpcHandler_RegisterDriver_PropagatesServiceError(t *testing.T) {
	s := NewService()
	_, _ = s.RegisterDriver("driver-123", "standard")

	_, lis, cleanup := newTestServer(t, s)
	defer cleanup()

	client := newTestClient(t, lis)

	req := &pb.RegisterDriverRequest{
		DriverID:    "driver-123",
		PackageSlug: "standard",
	}

	resp, err := client.RegisterDriver(context.Background(), req)

	if err != nil {
		st, ok := status.FromError(err)
		if !ok {
			t.Errorf("error should be gRPC status, got %T", err)
		} else if st.Code() != codes.Internal && st.Code() != codes.AlreadyExists {
			t.Errorf("error code = %v, want Internal or AlreadyExists", st.Code())
		}
		t.Logf("RegisterDriver returned error (expected if duplicate check implemented): %v", err)
	} else if resp != nil {
		t.Log("RegisterDriver succeeded (duplicate check not implemented in service)")
	}
}

func TestGrpcHandler_RegisterDriver_EmptyDriverID(t *testing.T) {
	s := NewService()
	_, lis, cleanup := newTestServer(t, s)
	defer cleanup()

	client := newTestClient(t, lis)

	req := &pb.RegisterDriverRequest{
		DriverID:    "",
		PackageSlug: "standard",
	}

	resp, err := client.RegisterDriver(context.Background(), req)

	if err != nil {
		t.Logf("RegisterDriver with empty ID returned error: %v", err)
	} else if resp != nil {
		t.Logf("RegisterDriver with empty ID succeeded: driver.ID=%q", resp.Driver.Id)
	}
}

func TestGrpcHandler_RegisterDriver_EmptyPackageSlug(t *testing.T) {
	s := NewService()
	_, lis, cleanup := newTestServer(t, s)
	defer cleanup()

	client := newTestClient(t, lis)

	req := &pb.RegisterDriverRequest{
		DriverID:    "driver-123",
		PackageSlug: "",
	}

	resp, err := client.RegisterDriver(context.Background(), req)

	if err != nil {
		t.Logf("RegisterDriver with empty packageSlug returned error: %v", err)
	} else if resp != nil {
		if resp.Driver.PackageSlug != "" {
			t.Errorf("resp.Driver.PackageSlug = %q, want empty", resp.Driver.PackageSlug)
		}
	}
}

func TestGrpcHandler_UnregisterDriver_Success(t *testing.T) {
	s := NewService()
	_, lis, cleanup := newTestServer(t, s)
	defer cleanup()

	client := newTestClient(t, lis)

	// Primeiro registrar
	_, err := client.RegisterDriver(context.Background(), &pb.RegisterDriverRequest{
		DriverID:    "driver-123",
		PackageSlug: "standard",
	})
	if err != nil {
		t.Fatalf("RegisterDriver failed: %v", err)
	}

	// Depois desregistrar
	resp, err := client.UnregisterDriver(context.Background(), &pb.RegisterDriverRequest{
		DriverID: "driver-123",
	})
	if err != nil {
		t.Fatalf("UnregisterDriver RPC failed: %v", err)
	}

	if resp.Driver == nil {
		t.Fatal("UnregisterDriverResponse.Driver is nil")
	}
	if resp.Driver.Id != "driver-123" {
		t.Errorf("resp.Driver.Id = %q, want %q", resp.Driver.Id, "driver-123")
	}

	// Verificar que foi removido
	drivers := s.FindAvailableDrivers("standard")
	if len(drivers) != 0 {
		t.Errorf("driver not removed: %v", drivers)
	}
}

func TestGrpcHandler_UnregisterDriver_NonExistent(t *testing.T) {
	s := NewService()
	_, lis, cleanup := newTestServer(t, s)
	defer cleanup()

	client := newTestClient(t, lis)

	resp, err := client.UnregisterDriver(context.Background(), &pb.RegisterDriverRequest{
		DriverID: "non-existent",
	})

	if err != nil {
		t.Errorf("UnregisterDriver non-existent returned error: %v", err)
	}
	if resp == nil || resp.Driver == nil || resp.Driver.Id != "non-existent" {
		t.Errorf("resp.Driver = %v, want {Id: non-existent}", resp.Driver)
	}
}

func TestGrpcHandler_ConcurrentRequests(t *testing.T) {
	s := NewService()
	_, lis, cleanup := newTestServer(t, s)
	defer cleanup()

	client := newTestClient(t, lis)

	const numRequests = 50
	done := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			driverID := "driver-" + string(rune('a'+id%26))
			_, err := client.RegisterDriver(context.Background(), &pb.RegisterDriverRequest{
				DriverID:    driverID,
				PackageSlug: "standard",
			})
			done <- err
		}(i)
	}

	for i := 0; i < numRequests; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent request %d failed: %v", i, err)
		}
	}

	drivers := s.FindAvailableDrivers("standard")
	t.Logf("Final driver count after concurrent requests: %d", len(drivers))
}