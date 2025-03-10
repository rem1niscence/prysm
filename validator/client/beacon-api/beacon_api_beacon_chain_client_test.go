package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api/mock"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestListValidators(t *testing.T) {
	const blockHeaderEndpoint = "/eth/v1/beacon/headers/head"

	t.Run("invalid token", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ctx := context.Background()

		beaconChainClient := beaconApiChainClient{}
		_, err := beaconChainClient.Validators(ctx, &ethpb.ListValidatorsRequest{
			PageToken: "foo",
		})
		assert.ErrorContains(t, "failed to parse page token `foo`", err)
	})

	t.Run("query filter epoch overflow", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ctx := context.Background()

		beaconChainClient := beaconApiChainClient{}
		_, err := beaconChainClient.Validators(ctx, &ethpb.ListValidatorsRequest{
			QueryFilter: &ethpb.ListValidatorsRequest_Epoch{
				Epoch: math.MaxUint64,
			},
		})
		assert.ErrorContains(t, fmt.Sprintf("failed to get first slot for epoch `%d`", uint64(math.MaxUint64)), err)
	})

	t.Run("fails to get validators for epoch filter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ctx := context.Background()

		stateValidatorsProvider := mock.NewMockStateValidatorsProvider(ctrl)
		stateValidatorsProvider.EXPECT().StateValidatorsForSlot(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			nil,
			errors.New("foo error"),
		)

		beaconChainClient := beaconApiChainClient{stateValidatorsProvider: stateValidatorsProvider}
		_, err := beaconChainClient.Validators(ctx, &ethpb.ListValidatorsRequest{
			QueryFilter: &ethpb.ListValidatorsRequest_Epoch{
				Epoch: 0,
			},
		})
		assert.ErrorContains(t, "failed to get state validators for slot `0`: foo error", err)
	})

	t.Run("fails to get validators for genesis filter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ctx := context.Background()

		stateValidatorsProvider := mock.NewMockStateValidatorsProvider(ctrl)
		stateValidatorsProvider.EXPECT().StateValidatorsForSlot(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			nil,
			errors.New("bar error"),
		)

		beaconChainClient := beaconApiChainClient{stateValidatorsProvider: stateValidatorsProvider}
		_, err := beaconChainClient.Validators(ctx, &ethpb.ListValidatorsRequest{
			QueryFilter: &ethpb.ListValidatorsRequest_Genesis{},
		})
		assert.ErrorContains(t, "failed to get genesis state validators: bar error", err)
	})

	t.Run("fails to get validators for nil filter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ctx := context.Background()

		stateValidatorsProvider := mock.NewMockStateValidatorsProvider(ctrl)
		stateValidatorsProvider.EXPECT().StateValidatorsForHead(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			nil,
			errors.New("foo error"),
		)

		beaconChainClient := beaconApiChainClient{stateValidatorsProvider: stateValidatorsProvider}
		_, err := beaconChainClient.Validators(ctx, &ethpb.ListValidatorsRequest{
			QueryFilter: nil,
		})
		assert.ErrorContains(t, "failed to get head state validators: foo error", err)
	})

	t.Run("fails to get latest block header for nil filter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ctx := context.Background()

		stateValidatorsProvider := mock.NewMockStateValidatorsProvider(ctrl)
		stateValidatorsProvider.EXPECT().StateValidatorsForHead(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			nil,
			nil,
		)

		jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
		jsonRestHandler.EXPECT().Get(gomock.Any(), blockHeaderEndpoint, gomock.Any()).Return(errors.New("bar error"))

		beaconChainClient := beaconApiChainClient{
			stateValidatorsProvider: stateValidatorsProvider,
			jsonRestHandler:         jsonRestHandler,
		}
		_, err := beaconChainClient.Validators(ctx, &ethpb.ListValidatorsRequest{
			QueryFilter: nil,
		})
		assert.ErrorContains(t, "bar error", err)
	})

	t.Run("fails to read block header response", func(t *testing.T) {
		testCases := []struct {
			name                string
			expectedError       string
			blockHeaderResponse structs.GetBlockHeaderResponse
		}{
			{
				name: "nil data",
				blockHeaderResponse: structs.GetBlockHeaderResponse{
					Data: nil,
				},
				expectedError: "block header data is nil",
			},
			{
				name: "nil data header",
				blockHeaderResponse: structs.GetBlockHeaderResponse{
					Data: &structs.SignedBeaconBlockHeaderContainer{
						Header: nil,
					},
				},
				expectedError: "block header data is nil",
			},
			{
				name: "nil message",
				blockHeaderResponse: structs.GetBlockHeaderResponse{
					Data: &structs.SignedBeaconBlockHeaderContainer{
						Header: &structs.SignedBeaconBlockHeader{
							Message: nil,
						},
					},
				},
				expectedError: "block header message is nil",
			},
			{
				name: "invalid header slot",
				blockHeaderResponse: structs.GetBlockHeaderResponse{
					Data: &structs.SignedBeaconBlockHeaderContainer{
						Header: &structs.SignedBeaconBlockHeader{
							Message: &structs.BeaconBlockHeader{
								Slot: "foo",
							},
						},
					},
				},
				expectedError: "failed to parse header slot `foo`",
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				ctx := context.Background()

				stateValidatorsProvider := mock.NewMockStateValidatorsProvider(ctrl)
				stateValidatorsProvider.EXPECT().StateValidatorsForHead(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
					nil,
					nil,
				)

				jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
				jsonRestHandler.EXPECT().Get(gomock.Any(), blockHeaderEndpoint, gomock.Any()).Return(
					nil,
				).SetArg(
					2,
					testCase.blockHeaderResponse,
				)

				beaconChainClient := beaconApiChainClient{
					stateValidatorsProvider: stateValidatorsProvider,
					jsonRestHandler:         jsonRestHandler,
				}
				_, err := beaconChainClient.Validators(ctx, &ethpb.ListValidatorsRequest{
					QueryFilter: nil,
				})
				assert.ErrorContains(t, testCase.expectedError, err)
			})
		}
	})

	t.Run("fails to get validators for genesis filter", func(t *testing.T) {
		generateValidStateValidatorsResponse := func() *structs.GetValidatorsResponse {
			return &structs.GetValidatorsResponse{
				Data: []*structs.ValidatorContainer{
					{
						Index: "1",
						Validator: &structs.Validator{
							Pubkey:                     hexutil.Encode([]byte{3}),
							WithdrawalCredentials:      hexutil.Encode([]byte{4}),
							EffectiveBalance:           "5",
							Slashed:                    true,
							ActivationEligibilityEpoch: "6",
							ActivationEpoch:            "7",
							ExitEpoch:                  "8",
							WithdrawableEpoch:          "9",
						},
					},
				},
			}
		}

		testCases := []struct {
			name                            string
			generateStateValidatorsResponse func() *structs.GetValidatorsResponse
			expectedError                   string
		}{
			{
				name: "nil validator",
				generateStateValidatorsResponse: func() *structs.GetValidatorsResponse {
					validatorsResponse := generateValidStateValidatorsResponse()
					validatorsResponse.Data[0].Validator = nil
					return validatorsResponse
				},
				expectedError: "state validator at index `0` is nil",
			},
			{
				name: "invalid pubkey",
				generateStateValidatorsResponse: func() *structs.GetValidatorsResponse {
					validatorsResponse := generateValidStateValidatorsResponse()
					validatorsResponse.Data[0].Validator.Pubkey = "foo"
					return validatorsResponse
				},
				expectedError: "failed to decode validator pubkey `foo`",
			},
			{
				name: "invalid withdrawal credentials",
				generateStateValidatorsResponse: func() *structs.GetValidatorsResponse {
					validatorsResponse := generateValidStateValidatorsResponse()
					validatorsResponse.Data[0].Validator.WithdrawalCredentials = "bar"
					return validatorsResponse
				},
				expectedError: "failed to decode validator withdrawal credentials `bar`",
			},
			{
				name: "invalid effective balance",
				generateStateValidatorsResponse: func() *structs.GetValidatorsResponse {
					validatorsResponse := generateValidStateValidatorsResponse()
					validatorsResponse.Data[0].Validator.EffectiveBalance = "foo"
					return validatorsResponse
				},
				expectedError: "failed to parse validator effective balance `foo`",
			},
			{
				name: "invalid validator index",
				generateStateValidatorsResponse: func() *structs.GetValidatorsResponse {
					validatorsResponse := generateValidStateValidatorsResponse()
					validatorsResponse.Data[0].Index = "bar"
					return validatorsResponse
				},
				expectedError: "failed to parse validator index `bar`",
			},
			{
				name: "invalid activation eligibility epoch",
				generateStateValidatorsResponse: func() *structs.GetValidatorsResponse {
					validatorsResponse := generateValidStateValidatorsResponse()
					validatorsResponse.Data[0].Validator.ActivationEligibilityEpoch = "foo"
					return validatorsResponse
				},
				expectedError: "failed to parse validator activation eligibility epoch `foo`",
			},
			{
				name: "invalid activation epoch",
				generateStateValidatorsResponse: func() *structs.GetValidatorsResponse {
					validatorsResponse := generateValidStateValidatorsResponse()
					validatorsResponse.Data[0].Validator.ActivationEpoch = "bar"
					return validatorsResponse
				},
				expectedError: "failed to parse validator activation epoch `bar`",
			},
			{
				name: "invalid exit epoch",
				generateStateValidatorsResponse: func() *structs.GetValidatorsResponse {
					validatorsResponse := generateValidStateValidatorsResponse()
					validatorsResponse.Data[0].Validator.ExitEpoch = "foo"
					return validatorsResponse
				},
				expectedError: "failed to parse validator exit epoch `foo`",
			},
			{
				name: "invalid withdrawable epoch",
				generateStateValidatorsResponse: func() *structs.GetValidatorsResponse {
					validatorsResponse := generateValidStateValidatorsResponse()
					validatorsResponse.Data[0].Validator.WithdrawableEpoch = "bar"
					return validatorsResponse
				},
				expectedError: "failed to parse validator withdrawable epoch `bar`",
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				ctx := context.Background()

				stateValidatorsProvider := mock.NewMockStateValidatorsProvider(ctrl)
				stateValidatorsProvider.EXPECT().StateValidatorsForSlot(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
					testCase.generateStateValidatorsResponse(),
					nil,
				)

				beaconChainClient := beaconApiChainClient{stateValidatorsProvider: stateValidatorsProvider}
				_, err := beaconChainClient.Validators(ctx, &ethpb.ListValidatorsRequest{
					QueryFilter: &ethpb.ListValidatorsRequest_Genesis{},
				})
				assert.ErrorContains(t, testCase.expectedError, err)
			})
		}
	})

	t.Run("correctly returns the expected validators", func(t *testing.T) {
		generateValidStateValidatorsResponse := func() *structs.GetValidatorsResponse {
			return &structs.GetValidatorsResponse{
				Data: []*structs.ValidatorContainer{
					{
						Index: "1",
						Validator: &structs.Validator{
							Pubkey:                     hexutil.Encode([]byte{2}),
							WithdrawalCredentials:      hexutil.Encode([]byte{3}),
							EffectiveBalance:           "4",
							Slashed:                    true,
							ActivationEligibilityEpoch: "5",
							ActivationEpoch:            "6",
							ExitEpoch:                  "7",
							WithdrawableEpoch:          "8",
						},
					},
					{
						Index: "9",
						Validator: &structs.Validator{
							Pubkey:                     hexutil.Encode([]byte{10}),
							WithdrawalCredentials:      hexutil.Encode([]byte{11}),
							EffectiveBalance:           "12",
							Slashed:                    false,
							ActivationEligibilityEpoch: "13",
							ActivationEpoch:            "14",
							ExitEpoch:                  "15",
							WithdrawableEpoch:          "16",
						},
					},
				},
			}
		}

		testCases := []struct {
			name                                string
			generateJsonStateValidatorsResponse func() *structs.GetValidatorsResponse
			generateProtoValidatorsResponse     func() *ethpb.Validators
			pubkeys                             [][]byte
			pubkeyStrings                       []string
			indices                             []primitives.ValidatorIndex
			statuses                            []string
			pageSize                            int32
			pageToken                           string
		}{
			{
				name: "page size 0",
				generateJsonStateValidatorsResponse: func() *structs.GetValidatorsResponse {
					validValidatorsResponse := generateValidStateValidatorsResponse()

					// Generate more than 250 validators, but expect only 250 to be returned
					validators := make([]*structs.ValidatorContainer, 267)
					for idx := 0; idx < len(validators); idx++ {
						validators[idx] = validValidatorsResponse.Data[0]
					}

					validatorsResponse := &structs.GetValidatorsResponse{
						Data: validators,
					}

					return validatorsResponse
				},
				generateProtoValidatorsResponse: func() *ethpb.Validators {
					validators := make([]*ethpb.Validators_ValidatorContainer, 250)
					for idx := 0; idx < len(validators); idx++ {
						validators[idx] = &ethpb.Validators_ValidatorContainer{
							Index: 1,
							Validator: &ethpb.Validator{
								PublicKey:                  []byte{2},
								WithdrawalCredentials:      []byte{3},
								EffectiveBalance:           4,
								Slashed:                    true,
								ActivationEligibilityEpoch: 5,
								ActivationEpoch:            6,
								ExitEpoch:                  7,
								WithdrawableEpoch:          8,
							},
						}
					}

					return &ethpb.Validators{
						ValidatorList: validators,
						TotalSize:     267,
						Epoch:         0,
						NextPageToken: "1",
					}
				},
				pubkeys:       [][]byte{},
				pubkeyStrings: make([]string, 0),
				indices:       []primitives.ValidatorIndex{},
				statuses:      nil,
				pageSize:      0,
				pageToken:     "",
			},
			{
				name:                                "pageSize==1 and pageToken==0",
				generateJsonStateValidatorsResponse: generateValidStateValidatorsResponse,
				generateProtoValidatorsResponse: func() *ethpb.Validators {
					return &ethpb.Validators{
						ValidatorList: []*ethpb.Validators_ValidatorContainer{
							{
								Index: 1,
								Validator: &ethpb.Validator{
									PublicKey:                  []byte{2},
									WithdrawalCredentials:      []byte{3},
									EffectiveBalance:           4,
									Slashed:                    true,
									ActivationEligibilityEpoch: 5,
									ActivationEpoch:            6,
									ExitEpoch:                  7,
									WithdrawableEpoch:          8,
								},
							},
						},
						TotalSize:     2,
						Epoch:         0,
						NextPageToken: "1",
					}
				},
				pageSize:  1,
				pageToken: "0",
			},
			{
				name:                                "pageSize==2 and pageToken==0",
				generateJsonStateValidatorsResponse: generateValidStateValidatorsResponse,
				generateProtoValidatorsResponse: func() *ethpb.Validators {
					return &ethpb.Validators{
						ValidatorList: []*ethpb.Validators_ValidatorContainer{
							{
								Index: 1,
								Validator: &ethpb.Validator{
									PublicKey:                  []byte{2},
									WithdrawalCredentials:      []byte{3},
									EffectiveBalance:           4,
									Slashed:                    true,
									ActivationEligibilityEpoch: 5,
									ActivationEpoch:            6,
									ExitEpoch:                  7,
									WithdrawableEpoch:          8,
								},
							},
							{
								Index: 9,
								Validator: &ethpb.Validator{
									PublicKey:                  []byte{10},
									WithdrawalCredentials:      []byte{11},
									EffectiveBalance:           12,
									Slashed:                    false,
									ActivationEligibilityEpoch: 13,
									ActivationEpoch:            14,
									ExitEpoch:                  15,
									WithdrawableEpoch:          16,
								},
							},
						},
						TotalSize:     2,
						Epoch:         0,
						NextPageToken: "",
					}
				},
				pageSize:  2,
				pageToken: "0",
			},
			{
				name:                                "pageSize==1 and pageToken==1",
				generateJsonStateValidatorsResponse: generateValidStateValidatorsResponse,
				generateProtoValidatorsResponse: func() *ethpb.Validators {
					return &ethpb.Validators{
						ValidatorList: []*ethpb.Validators_ValidatorContainer{
							{
								Index: 9,
								Validator: &ethpb.Validator{
									PublicKey:                  []byte{10},
									WithdrawalCredentials:      []byte{11},
									EffectiveBalance:           12,
									Slashed:                    false,
									ActivationEligibilityEpoch: 13,
									ActivationEpoch:            14,
									ExitEpoch:                  15,
									WithdrawableEpoch:          16,
								},
							},
						},
						TotalSize:     2,
						Epoch:         0,
						NextPageToken: "",
					}
				},
				pageSize:  1,
				pageToken: "1",
			},
			{
				name:                                "pageSize==1 and pageToken==2",
				generateJsonStateValidatorsResponse: generateValidStateValidatorsResponse,
				generateProtoValidatorsResponse: func() *ethpb.Validators {
					return &ethpb.Validators{
						ValidatorList: []*ethpb.Validators_ValidatorContainer{},
						TotalSize:     2,
						Epoch:         0,
						NextPageToken: "",
					}
				},
				pageSize:  1,
				pageToken: "2",
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				ctx := context.Background()

				stateValidatorsProvider := mock.NewMockStateValidatorsProvider(ctrl)
				stateValidatorsProvider.EXPECT().StateValidatorsForSlot(gomock.Any(), primitives.Slot(0), make([]string, 0), []primitives.ValidatorIndex{}, nil).Return(
					testCase.generateJsonStateValidatorsResponse(),
					nil,
				)

				beaconChainClient := beaconApiChainClient{stateValidatorsProvider: stateValidatorsProvider}
				validators, err := beaconChainClient.Validators(ctx, &ethpb.ListValidatorsRequest{
					QueryFilter: &ethpb.ListValidatorsRequest_Genesis{},
					PublicKeys:  [][]byte{},
					Indices:     []primitives.ValidatorIndex{},
					Active:      false,
					PageSize:    testCase.pageSize,
					PageToken:   testCase.pageToken,
				})
				require.NoError(t, err)
				require.NotNil(t, validators)

				expectedValidators := testCase.generateProtoValidatorsResponse()
				assert.DeepEqual(t, expectedValidators, validators)
			})
		}
	})
}

func TestGetChainHead(t *testing.T) {
	const finalityCheckpointsEndpoint = "/eth/v1/beacon/states/head/finality_checkpoints"
	const headBlockHeadersEndpoint = "/eth/v1/beacon/headers/head"

	generateValidFinalityCheckpointsResponse := func() structs.GetFinalityCheckpointsResponse {
		return structs.GetFinalityCheckpointsResponse{
			Data: &structs.FinalityCheckpoints{
				PreviousJustified: &structs.Checkpoint{
					Epoch: "1",
					Root:  hexutil.Encode([]byte{2}),
				},
				CurrentJustified: &structs.Checkpoint{
					Epoch: "3",
					Root:  hexutil.Encode([]byte{4}),
				},
				Finalized: &structs.Checkpoint{
					Epoch: "5",
					Root:  hexutil.Encode([]byte{6}),
				},
			},
		}
	}

	t.Run("fails to get finality checkpoints", func(t *testing.T) {
		testCases := []struct {
			name                                string
			generateFinalityCheckpointsResponse func() structs.GetFinalityCheckpointsResponse
			finalityCheckpointsError            error
			expectedError                       string
		}{
			{
				name:                     "query failed",
				finalityCheckpointsError: errors.New("foo error"),
				expectedError:            "foo error",
				generateFinalityCheckpointsResponse: func() structs.GetFinalityCheckpointsResponse {
					return structs.GetFinalityCheckpointsResponse{}
				},
			},
			{
				name:          "nil finality checkpoints data",
				expectedError: "finality checkpoints data is nil",
				generateFinalityCheckpointsResponse: func() structs.GetFinalityCheckpointsResponse {
					validResponse := generateValidFinalityCheckpointsResponse()
					validResponse.Data = nil
					return validResponse
				},
			},
			{
				name:          "nil finalized checkpoint",
				expectedError: "finalized checkpoint is nil",
				generateFinalityCheckpointsResponse: func() structs.GetFinalityCheckpointsResponse {
					validResponse := generateValidFinalityCheckpointsResponse()
					validResponse.Data.Finalized = nil
					return validResponse
				},
			},
			{
				name:          "invalid finalized epoch",
				expectedError: "failed to parse finalized epoch `foo`",
				generateFinalityCheckpointsResponse: func() structs.GetFinalityCheckpointsResponse {
					validResponse := generateValidFinalityCheckpointsResponse()
					validResponse.Data.Finalized.Epoch = "foo"
					return validResponse
				},
			},
			{
				name:          "failed to get first slot of finalized epoch",
				expectedError: fmt.Sprintf("failed to get first slot for epoch `%d`", uint64(math.MaxUint64)),
				generateFinalityCheckpointsResponse: func() structs.GetFinalityCheckpointsResponse {
					validResponse := generateValidFinalityCheckpointsResponse()
					validResponse.Data.Finalized.Epoch = strconv.FormatUint(uint64(math.MaxUint64), 10)
					return validResponse
				},
			},
			{
				name:          "invalid finalized root",
				expectedError: "failed to decode finalized checkpoint root `bar`",
				generateFinalityCheckpointsResponse: func() structs.GetFinalityCheckpointsResponse {
					validResponse := generateValidFinalityCheckpointsResponse()
					validResponse.Data.Finalized.Root = "bar"
					return validResponse
				},
			},
			{
				name:          "nil current justified checkpoint",
				expectedError: "current justified checkpoint is nil",
				generateFinalityCheckpointsResponse: func() structs.GetFinalityCheckpointsResponse {
					validResponse := generateValidFinalityCheckpointsResponse()
					validResponse.Data.CurrentJustified = nil
					return validResponse
				},
			},
			{
				name:          "nil current justified epoch",
				expectedError: "failed to parse current justified checkpoint epoch `foo`",
				generateFinalityCheckpointsResponse: func() structs.GetFinalityCheckpointsResponse {
					validResponse := generateValidFinalityCheckpointsResponse()
					validResponse.Data.CurrentJustified.Epoch = "foo"
					return validResponse
				},
			},
			{
				name:          "failed to get first slot of current justified epoch",
				expectedError: fmt.Sprintf("failed to get first slot for epoch `%d`", uint64(math.MaxUint64)),
				generateFinalityCheckpointsResponse: func() structs.GetFinalityCheckpointsResponse {
					validResponse := generateValidFinalityCheckpointsResponse()
					validResponse.Data.CurrentJustified.Epoch = strconv.FormatUint(uint64(math.MaxUint64), 10)
					return validResponse
				},
			},
			{
				name:          "invalid current justified root",
				expectedError: "failed to decode current justified checkpoint root `bar`",
				generateFinalityCheckpointsResponse: func() structs.GetFinalityCheckpointsResponse {
					validResponse := generateValidFinalityCheckpointsResponse()
					validResponse.Data.CurrentJustified.Root = "bar"
					return validResponse
				},
			},
			{
				name:          "nil previous justified checkpoint",
				expectedError: "previous justified checkpoint is nil",
				generateFinalityCheckpointsResponse: func() structs.GetFinalityCheckpointsResponse {
					validResponse := generateValidFinalityCheckpointsResponse()
					validResponse.Data.PreviousJustified = nil
					return validResponse
				},
			},
			{
				name:          "nil previous justified epoch",
				expectedError: "failed to parse previous justified checkpoint epoch `foo`",
				generateFinalityCheckpointsResponse: func() structs.GetFinalityCheckpointsResponse {
					validResponse := generateValidFinalityCheckpointsResponse()
					validResponse.Data.PreviousJustified.Epoch = "foo"
					return validResponse
				},
			},
			{
				name:          "failed to get first slot of previous justified epoch",
				expectedError: fmt.Sprintf("failed to get first slot for epoch `%d`", uint64(math.MaxUint64)),
				generateFinalityCheckpointsResponse: func() structs.GetFinalityCheckpointsResponse {
					validResponse := generateValidFinalityCheckpointsResponse()
					validResponse.Data.PreviousJustified.Epoch = strconv.FormatUint(uint64(math.MaxUint64), 10)
					return validResponse
				},
			},
			{
				name:          "invalid previous justified root",
				expectedError: "failed to decode previous justified checkpoint root `bar`",
				generateFinalityCheckpointsResponse: func() structs.GetFinalityCheckpointsResponse {
					validResponse := generateValidFinalityCheckpointsResponse()
					validResponse.Data.PreviousJustified.Root = "bar"
					return validResponse
				},
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				ctx := context.Background()

				finalityCheckpointsResponse := structs.GetFinalityCheckpointsResponse{}
				jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
				jsonRestHandler.EXPECT().Get(gomock.Any(), finalityCheckpointsEndpoint, &finalityCheckpointsResponse).Return(
					testCase.finalityCheckpointsError,
				).SetArg(
					2,
					testCase.generateFinalityCheckpointsResponse(),
				)

				beaconChainClient := beaconApiChainClient{jsonRestHandler: jsonRestHandler}
				_, err := beaconChainClient.ChainHead(ctx, &emptypb.Empty{})
				assert.ErrorContains(t, testCase.expectedError, err)
			})
		}
	})

	generateValidBlockHeadersResponse := func() structs.GetBlockHeaderResponse {
		return structs.GetBlockHeaderResponse{
			Data: &structs.SignedBeaconBlockHeaderContainer{
				Root: hexutil.Encode([]byte{7}),
				Header: &structs.SignedBeaconBlockHeader{
					Message: &structs.BeaconBlockHeader{
						Slot: "8",
					},
				},
			},
		}
	}

	t.Run("fails to get head block headers", func(t *testing.T) {
		testCases := []struct {
			name                             string
			generateHeadBlockHeadersResponse func() structs.GetBlockHeaderResponse
			headBlockHeadersError            error
			expectedError                    string
		}{
			{
				name:                  "query failed",
				headBlockHeadersError: errors.New("foo error"),
				expectedError:         "failed to get head block header",
				generateHeadBlockHeadersResponse: func() structs.GetBlockHeaderResponse {
					return structs.GetBlockHeaderResponse{}
				},
			},
			{
				name:          "nil block header data",
				expectedError: "block header data is nil",
				generateHeadBlockHeadersResponse: func() structs.GetBlockHeaderResponse {
					validResponse := generateValidBlockHeadersResponse()
					validResponse.Data = nil
					return validResponse
				},
			},
			{
				name:          "nil block header data header",
				expectedError: "block header data is nil",
				generateHeadBlockHeadersResponse: func() structs.GetBlockHeaderResponse {
					validResponse := generateValidBlockHeadersResponse()
					validResponse.Data.Header = nil
					return validResponse
				},
			},
			{
				name:          "nil block header message",
				expectedError: "block header message is nil",
				generateHeadBlockHeadersResponse: func() structs.GetBlockHeaderResponse {
					validResponse := generateValidBlockHeadersResponse()
					validResponse.Data.Header.Message = nil
					return validResponse
				},
			},
			{
				name:          "invalid message slot",
				expectedError: "failed to parse head block slot `foo`",
				generateHeadBlockHeadersResponse: func() structs.GetBlockHeaderResponse {
					validResponse := generateValidBlockHeadersResponse()
					validResponse.Data.Header.Message.Slot = "foo"
					return validResponse
				},
			},

			{
				name:          "invalid root",
				expectedError: "failed to decode head block root `bar`",
				generateHeadBlockHeadersResponse: func() structs.GetBlockHeaderResponse {
					validResponse := generateValidBlockHeadersResponse()
					validResponse.Data.Root = "bar"
					return validResponse
				},
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				ctx := context.Background()

				jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

				finalityCheckpointsResponse := structs.GetFinalityCheckpointsResponse{}
				jsonRestHandler.EXPECT().Get(gomock.Any(), finalityCheckpointsEndpoint, &finalityCheckpointsResponse).Return(
					nil,
				).SetArg(
					2,
					generateValidFinalityCheckpointsResponse(),
				)

				headBlockHeadersResponse := structs.GetBlockHeaderResponse{}
				jsonRestHandler.EXPECT().Get(gomock.Any(), headBlockHeadersEndpoint, &headBlockHeadersResponse).Return(
					testCase.headBlockHeadersError,
				).SetArg(
					2,
					testCase.generateHeadBlockHeadersResponse(),
				)

				beaconChainClient := beaconApiChainClient{jsonRestHandler: jsonRestHandler}
				_, err := beaconChainClient.ChainHead(ctx, &emptypb.Empty{})
				assert.ErrorContains(t, testCase.expectedError, err)
			})
		}
	})

	t.Run("returns a valid chain head", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ctx := context.Background()

		jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

		finalityCheckpointsResponse := structs.GetFinalityCheckpointsResponse{}
		jsonRestHandler.EXPECT().Get(gomock.Any(), finalityCheckpointsEndpoint, &finalityCheckpointsResponse).Return(
			nil,
		).SetArg(
			2,
			generateValidFinalityCheckpointsResponse(),
		)

		headBlockHeadersResponse := structs.GetBlockHeaderResponse{}
		jsonRestHandler.EXPECT().Get(gomock.Any(), headBlockHeadersEndpoint, &headBlockHeadersResponse).Return(
			nil,
		).SetArg(
			2,
			generateValidBlockHeadersResponse(),
		)

		expectedPreviousJustifiedSlot, err := slots.EpochStart(1)
		require.NoError(t, err)

		expectedCurrentJustifiedSlot, err := slots.EpochStart(3)
		require.NoError(t, err)

		expectedFinalizedSlot, err := slots.EpochStart(5)
		require.NoError(t, err)

		expectedChainHead := &ethpb.ChainHead{
			PreviousJustifiedEpoch:     1,
			PreviousJustifiedBlockRoot: []byte{2},
			PreviousJustifiedSlot:      expectedPreviousJustifiedSlot,
			JustifiedEpoch:             3,
			JustifiedBlockRoot:         []byte{4},
			JustifiedSlot:              expectedCurrentJustifiedSlot,
			FinalizedEpoch:             5,
			FinalizedBlockRoot:         []byte{6},
			FinalizedSlot:              expectedFinalizedSlot,
			HeadBlockRoot:              []byte{7},
			HeadSlot:                   8,
			HeadEpoch:                  slots.ToEpoch(8),
		}

		beaconChainClient := beaconApiChainClient{jsonRestHandler: jsonRestHandler}
		chainHead, err := beaconChainClient.ChainHead(ctx, &emptypb.Empty{})
		require.NoError(t, err)
		assert.DeepEqual(t, expectedChainHead, chainHead)
	})
}

func Test_beaconApiBeaconChainClient_GetValidatorPerformance(t *testing.T) {
	publicKeys := [][48]byte{
		bytesutil.ToBytes48([]byte{1}),
		bytesutil.ToBytes48([]byte{2}),
		bytesutil.ToBytes48([]byte{3}),
	}

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	request, err := json.Marshal(structs.GetValidatorPerformanceRequest{
		PublicKeys: [][]byte{publicKeys[0][:], publicKeys[2][:], publicKeys[1][:]},
	})
	require.NoError(t, err)

	wantResponse := &structs.GetValidatorPerformanceResponse{}
	want := &ethpb.ValidatorPerformanceResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		getValidatorPerformanceEndpoint,
		nil,
		bytes.NewBuffer(request),
		wantResponse,
	).Return(
		nil,
	)

	c := beaconApiChainClient{
		jsonRestHandler: jsonRestHandler,
	}

	got, err := c.ValidatorPerformance(ctx, &ethpb.ValidatorPerformanceRequest{
		PublicKeys: [][]byte{publicKeys[0][:], publicKeys[2][:], publicKeys[1][:]},
	})
	require.NoError(t, err)
	require.DeepEqual(t, want.PublicKeys, got.PublicKeys)
}
