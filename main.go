package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "keda-horizon-scaler/externalscaler"
)

// ExternalScalerServer implements the gRPC service defined in externalscaler.proto
type ExternalScalerServer struct {
	pb.UnimplementedExternalScalerServer
}

func getMetadata(md map[string]string, key, def string) string {
	if v, ok := md[key]; ok && v != "" {
		return v
	}
	return def
}

// IsActive: active if there’s at least one pending job
func (s *ExternalScalerServer) IsActive(ctx context.Context, ref *pb.ScaledObjectRef) (*pb.IsActiveResponse, error) {
	md := ref.GetScalerMetadata()

	addr := getMetadata(md, "redisAddress", "")
	pass := getMetadata(md, "redisPassword", "")
	dbNum, _ := strconv.Atoi(getMetadata(md, "redisDB", "0"))
	prefix := getMetadata(md, "prefix", "") // e.g. "myapp_horizon:"
	if addr == "" || prefix == "" {
		return nil, status.Error(codes.InvalidArgument, "redisAddress and prefix are required metadata")
	}

	count, err := getPendingCount(ctx, addr, pass, dbNum, prefix)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error fetching pending count: %v", err)
	}
	return &pb.IsActiveResponse{Result: count > 0}, nil
}

// GetMetricSpec: tells KEDA the metric name and target threshold
func (s *ExternalScalerServer) GetMetricSpec(ctx context.Context, ref *pb.ScaledObjectRef) (*pb.GetMetricSpecResponse, error) {
	md := ref.GetScalerMetadata()
	threshold, err := strconv.Atoi(getMetadata(md, "threshold", "10"))
	if err != nil {
		threshold = 10
	}
	metricName := fmt.Sprintf("redis-pending-jobs-%s", md["prefix"])
	return &pb.GetMetricSpecResponse{
		MetricSpecs: []*pb.MetricSpec{{
			MetricName: metricName,
			TargetSize: int64(threshold),
		}},
	}, nil
}

// GetMetrics: returns current ZCOUNT of the sorted set
func (s *ExternalScalerServer) GetMetrics(ctx context.Context, req *pb.GetMetricsRequest) (*pb.GetMetricsResponse, error) {
	md := req.GetScaledObjectRef().GetScalerMetadata()

	addr := getMetadata(md, "redisAddress", "")
	pass := getMetadata(md, "redisPassword", "")
	dbNum, _ := strconv.Atoi(getMetadata(md, "redisDB", "0"))
	prefix := getMetadata(md, "prefix", "")

	count, err := getPendingCount(ctx, addr, pass, dbNum, prefix)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error fetching pending count: %v", err)
	}

	return &pb.GetMetricsResponse{
		MetricValues: []*pb.MetricValue{{
			MetricName:  req.GetMetricName(),
			MetricValue: int64(count),
		}},
	}, nil
}

// getPendingCount performs: ZCOUNT {prefix}pending_jobs -inf +inf
func getPendingCount(ctx context.Context, addr, pass string, db int, prefix string) (int, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: pass,
		DB:       db,
	})
	defer rdb.Close()

	key := prefix + "pending_jobs"
	count, err := rdb.ZCount(ctx, key, "-inf", "+inf").Result()
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func main() {
	port := flag.String("port", "6000", "gRPC listen port")
	flag.Parse()

	lis, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterExternalScalerServer(grpcServer, &ExternalScalerServer{})
	log.Printf("🔄 external scaler running on :%s", *port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
