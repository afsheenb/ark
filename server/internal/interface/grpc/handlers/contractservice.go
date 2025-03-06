package handlers

import (
	"context"
	"fmt"
	"time"

	pb "github.com/ark-network/ark/api-spec/protobuf/gen/ark/v1"
	"github.com/ark-network/ark/server/internal/core/application"
	"github.com/ark-network/ark/server/internal/core/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ContractServiceHandler handles gRPC requests for the hashrate contract service
type ContractServiceHandler struct {
	pb.UnimplementedContractServiceServer
	contractService    *application.ContractService
	hashrateCalculator *application.HashrateCalculator
}

// NewContractServiceHandler creates a new contract service handler
func NewContractServiceHandler(contractService *application.ContractService, 
	hashrateCalculator *application.HashrateCalculator) *ContractServiceHandler {
	return &ContractServiceHandler{
		contractService:    contractService,
		hashrateCalculator: hashrateCalculator,
	}
}

// CreateContract creates a new hashrate contract
func (h *ContractServiceHandler) CreateContract(
	ctx context.Context,
	req *pb.CreateContractRequest,
) (*pb.CreateContractResponse, error) {
	// Set default contract type (CALL) since it's not in the request
	contractType := domain.ContractTypeCall

	// Parse strike hash rate from string to float64
	strikeHashRate, err := parseStrikeHashRate(req.StrikeHashRate)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid strike hash rate: %v", err)
	}

	// Convert int32 to uint64 for contract size and premium
	contractSize := uint64(req.ContractSize)
	premium := uint64(req.Premium)

	// Create contract
	contract, err := h.contractService.CreateContract(
		contractType,
		strikeHashRate,
		req.StartBlockHeight,
		req.EndBlockHeight,
		req.TargetTimestamp,
		contractSize,
		premium,
		req.BuyerPubkey,
		req.SellerPubkey,
		req.ExpiresAt,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create contract: %v", err)
	}

	// Convert domain contract to proto
	pbContract := domainContractToProto(contract)

	return &pb.CreateContractResponse{
		Contract: pbContract,
	}, nil
}

// GetContract gets a contract by ID
func (h *ContractServiceHandler) GetContract(
	ctx context.Context,
	req *pb.GetContractRequest,
) (*pb.GetContractResponse, error) {
	// Get contract
	contract, err := h.contractService.GetContract(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "contract not found: %v", err)
	}

	// Convert domain contract to proto
	pbContract := domainContractToProto(contract)

	return &pb.GetContractResponse{
		Contract: pbContract,
	}, nil
}

// SetupContract sets up a contract with initial transaction
func (h *ContractServiceHandler) SetupContract(
	ctx context.Context,
	req *pb.SetupContractRequest,
) (*pb.SetupContractResponse, error) {
	// Setup contract
	contract, err := h.contractService.SetupContract(req.Id, req.SetupTx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to setup contract: %v", err)
	}

	return &pb.SetupContractResponse{
		SetupTxid:    contract.SetupTxid,
		SignedSetupTx: req.SetupTx, // In a real implementation, this would be the signed tx
	}, nil
}

// SettleContract settles a contract
func (h *ContractServiceHandler) SettleContract(
	ctx context.Context,
	req *pb.SettleContractRequest,
) (*pb.SettleContractResponse, error) {
	// Settle contract
	contract, winnerPubkey, err := h.contractService.SettleContract(req.Id, req.SettlementTx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to settle contract: %v", err)
	}

	return &pb.SettleContractResponse{
		SettlementTxid:    contract.SettlementTxid,
		SignedSettlementTx: req.SettlementTx, // In a real implementation, this would be the signed tx
		WinnerPubkey:      winnerPubkey,
	}, nil
}

// ListContracts lists contracts with optional filters
func (h *ContractServiceHandler) ListContracts(
	ctx context.Context,
	req *pb.ListContractsRequest,
) (*pb.ListContractsResponse, error) {
	// Since req.Status is not in ListContractsRequest, use empty string to indicate no status filter
	var statusFilter domain.ContractStatus
	statusFilter = "" // Empty string means no status filter

	// List contracts
	contracts, total, err := h.contractService.ListContracts(
		statusFilter,
		req.UserPubkey,
		int(req.Limit),
		int(req.Offset),
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list contracts: %v", err)
	}

	// Convert domain contracts to proto
	pbContracts := make([]*pb.HashRateContract, len(contracts))
	for i, contract := range contracts {
		pbContracts[i] = domainContractToProto(contract)
	}

	return &pb.ListContractsResponse{
		Contracts: pbContracts,
		Total:     int32(total),
	}, nil
}

// GetHashrate gets the current Bitcoin network hashrate
func (h *ContractServiceHandler) GetHashrate(
	ctx context.Context,
	req *pb.GetHashrateRequest,
) (*pb.GetHashrateResponse, error) {
	// Get hashrate
	hashrate, difficulty, blockHeight, timestamp, err := h.contractService.GetHashrate(req.WindowSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get hashrate: %v", err)
	}

	return &pb.GetHashrateResponse{
		Hashrate:     hashrate,
		Difficulty:   difficulty,
		BlockHeight:  blockHeight,
		Timestamp:    timestamp,
	}, nil
}

// GetHistoricalHashrates gets historical hashrate data
func (h *ContractServiceHandler) GetHistoricalHashrates(
	ctx context.Context,
	req *pb.GetHistoricalHashratesRequest,
) (*pb.GetHistoricalHashratesResponse, error) {
	// Get historical hashrates
	hashrateData, err := h.contractService.GetHistoricalHashrates(
		req.FromHeight,
		req.ToHeight,
		int(req.Limit),
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get historical hashrates: %v", err)
	}

	// Convert domain hashrate data to proto
	pbHashrates := make([]*pb.HistoricalHashrate, len(hashrateData))
	for i, data := range hashrateData {
		pbHashrates[i] = &pb.HistoricalHashrate{
			BlockHeight: data.BlockHeight,
			Timestamp:   data.Timestamp,
			Hashrate:    data.Hashrate,
			Difficulty:  data.Difficulty,
		}
	}

	return &pb.GetHistoricalHashratesResponse{
		Hashrates: pbHashrates,
	}, nil
}

// CheckSettlement checks if a contract can be settled
func (h *ContractServiceHandler) CheckSettlement(
	ctx context.Context,
	req *pb.CheckSettlementRequest,
) (*pb.CheckSettlementResponse, error) {
	// Check settlement
	canSettle, winnerPubkey, err := h.contractService.CheckSettlement(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check settlement: %v", err)
	}

	// If contract cannot be settled, provide a reason
	var reason string
	if !canSettle {
		reason = "Contract cannot be settled yet. Neither end block height nor target timestamp has been reached."
	}

	return &pb.CheckSettlementResponse{
		CanSettle:    canSettle,
		WinnerPubkey: winnerPubkey,
		Reason:       reason,
	}, nil
}

// GetContractEvents gets a stream of contract events
func (h *ContractServiceHandler) GetContractEvents(
	req *pb.GetContractEventsRequest,
	stream pb.ContractService_GetContractEventsServer,
) error {
	// Channel for events
	eventCh := make(chan interface{})
	defer close(eventCh)
	
	// Subscribe to events
	// This assumes EventPublisher is accessible from the ContractService
	// If not, you'll need to modify this to receive an EventPublisher in the constructor
	// In a real implementation, you would use the EventPublisher to subscribe to events
	
	// Mock implementation - send some test events
	// In a real implementation, this would be replaced with real subscription logic
	go func() {
		time.Sleep(1 * time.Second)
		
		// Mock a contract created event
		contract := &domain.HashrateContract{
			ID:               "mock-contract-id",
			ContractType:     domain.ContractTypeCall,
			StrikeHashRate:   350.0,
			StartBlockHeight: 800000,
			EndBlockHeight:   802016,
			TargetTimestamp:  time.Now().Add(14 * 24 * time.Hour).Unix(),
			ContractSize:     100000000,
			Premium:          5000000,
			BuyerPubkey:      "mock-buyer-pubkey",
			SellerPubkey:     "mock-seller-pubkey",
			Status:           domain.ContractStatusCreated,
			CreatedAt:        time.Now().Unix(),
			UpdatedAt:        time.Now().Unix(),
		}
		
		eventCh <- &pb.ContractCreatedEvent{
			Contract: domainContractToProto(contract),
		}
		
		time.Sleep(1 * time.Second)
		
		// Mock a contract setup event
		eventCh <- &pb.ContractSetupEvent{
			ContractId: "mock-contract-id",
			SetupTxid:  "mock-setup-txid",
		}
	}()
	
	// Send events to client
	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-eventCh:
			var pbEvent *pb.GetContractEventsResponse
			
			switch e := event.(type) {
			case *pb.ContractCreatedEvent:
				pbEvent = &pb.GetContractEventsResponse{
					Event: &pb.GetContractEventsResponse_ContractCreated{
						ContractCreated: e,
					},
				}
			case *pb.ContractSetupEvent:
				pbEvent = &pb.GetContractEventsResponse{
					Event: &pb.GetContractEventsResponse_ContractSetup{
						ContractSetup: e,
					},
				}
			case *pb.ContractSettledEvent:
				pbEvent = &pb.GetContractEventsResponse{
					Event: &pb.GetContractEventsResponse_ContractSettled{
						ContractSettled: e,
					},
				}
			default:
				continue
			}
			
			if err := stream.Send(pbEvent); err != nil {
				return err
			}
		}
	}
}

// Helper function to parse strike hash rate from string to float64
func parseStrikeHashRate(strikeHashRateStr string) (float64, error) {
	var strikeHashRate float64
	_, err := fmt.Sscanf(strikeHashRateStr, "%f", &strikeHashRate)
	if err != nil {
		return 0, err
	}
	return strikeHashRate, nil
}

// domainContractToProto converts a domain contract to a proto contract
func domainContractToProto(contract *domain.HashrateContract) *pb.HashRateContract {
	// Convert contract type
	var contractType pb.ContractType
	switch contract.ContractType {
	case domain.ContractTypeCall:
		contractType = pb.ContractType_CONTRACT_TYPE_CALL
	case domain.ContractTypePut:
		contractType = pb.ContractType_CONTRACT_TYPE_PUT
	default:
		contractType = pb.ContractType_CONTRACT_TYPE_UNSPECIFIED
	}

	// Convert status
	var status pb.ContractStatus
	switch contract.Status {
	case domain.ContractStatusCreated:
		status = pb.ContractStatus_CONTRACT_STATUS_CREATED
	case domain.ContractStatusSetup:
		status = pb.ContractStatus_CONTRACT_STATUS_SETUP
	case domain.ContractStatusActive:
		status = pb.ContractStatus_CONTRACT_STATUS_ACTIVE
	case domain.ContractStatusSettled:
		status = pb.ContractStatus_CONTRACT_STATUS_SETTLED
	case domain.ContractStatusExpired:
		status = pb.ContractStatus_CONTRACT_STATUS_EXPIRED
	case domain.ContractStatusFailed:
		status = pb.ContractStatus_CONTRACT_STATUS_FAILED
	default:
		status = pb.ContractStatus_CONTRACT_STATUS_UNSPECIFIED
	}

	return &pb.HashRateContract{
		Id:               contract.ID,
		ContractType:     contractType,
		StrikeHashRate:   contract.StrikeHashRate,
		StartBlockHeight: contract.StartBlockHeight,
		EndBlockHeight:   contract.EndBlockHeight,
		TargetTimestamp:  contract.TargetTimestamp,
		ContractSize:     contract.ContractSize,
		Premium:          contract.Premium,
		BuyerPubkey:      contract.BuyerPubkey,
		SellerPubkey:     contract.SellerPubkey,
		Status:           status,
		CreatedAt:        contract.CreatedAt,
		UpdatedAt:        contract.UpdatedAt,
		ExpiresAt:        contract.ExpiresAt,
		SetupTxid:        contract.SetupTxid,
		FinalTxid:        contract.FinalTxid,
		SettlementTxid:   contract.SettlementTxid,
	}
}