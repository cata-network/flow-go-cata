package verification

import (
	"fmt"

	"github.com/dapperlabs/flow-go/consensus/hotstuff/model"
	"github.com/dapperlabs/flow-go/model/flow"
	"github.com/dapperlabs/flow-go/model/flow/filter"
	"github.com/dapperlabs/flow-go/model/flow/order"
	"github.com/dapperlabs/flow-go/module"
	"github.com/dapperlabs/flow-go/state/dkg"
	"github.com/dapperlabs/flow-go/state/protocol"
)

// CombinedVerifier is a verifier capable of verifying two signatures for each
// verifying operation. The first type is a signature from an aggregating signer,
// which verifies either the single or the aggregated signature. The second type is
// a signature from a threshold signer, which verifies either the signature share or
// the reconstructed threshold signature.
type CombinedVerifier struct {
	state   protocol.State
	dkg     dkg.State
	staking module.AggregatingVerifier
	beacon  module.ThresholdVerifier
	merger  module.Merger
	filter  flow.IdentityFilter
}

// NewCombinedVerifier creates a new combined verifier with the given dependencies.
// - the protocol state is used to retrieve the public keys for the staking signature;
// - the DKG state is used to retrieve DKG data necessary to verify beacon signatures;
// - the staking verifier is used to verify single & aggregated staking signatures;
// - the beacon verifier is used to verify signature shares & threshold signatures;
// - the merger is used to combined & split staking & random beacon signatures; and
// - the filter is used to select the set of scheme participants from the protocol state.
func NewCombinedVerifier(state protocol.State, dkg dkg.State, staking module.AggregatingVerifier, beacon module.ThresholdVerifier, merger module.Merger, filter flow.IdentityFilter) *CombinedVerifier {
	c := &CombinedVerifier{
		state:   state,
		dkg:     dkg,
		staking: staking,
		beacon:  beacon,
		merger:  merger,
		filter:  filter,
	}
	return c
}

// VerifyVote verifies the validity of a combined signature on a vote.
func (c *CombinedVerifier) VerifyVote(vote *model.Vote) (bool, error) {

	// verify the signature data
	msg := messageFromParams(vote.View, vote.BlockID)
	valid, err := c.verifySigData(vote.BlockID, msg, vote.SigData, vote.SignerID)
	if err != nil {
		return false, fmt.Errorf("could not verify signature: %w", err)
	}

	return valid, nil
}

// VerifyProposal verifies the validity of a combined signature on a proposal.
func (c *CombinedVerifier) VerifyProposal(proposal *model.Proposal) (bool, error) {

	// verify the signature data
	msg := messageFromParams(proposal.Block.View, proposal.Block.BlockID)
	valid, err := c.verifySigData(proposal.Block.BlockID, msg, proposal.SigData, proposal.Block.ProposerID)
	if err != nil {
		return false, fmt.Errorf("could not verify signature: %w", err)
	}

	return valid, nil
}

// VerifyQC verifies the validity of a combined signature on a quorum certificate.
func (c *CombinedVerifier) VerifyQC(qc *model.QuorumCertificate) (bool, error) {

	// get the participants of the signature scheme
	participants, err := c.state.AtBlockID(qc.BlockID).Identities(c.filter)
	if err != nil {
		return false, fmt.Errorf("could not get signer identities: %w", err)
	}

	// get the DKG group key from the DKG state
	dkgKey, err := c.dkg.GroupKey()
	if err != nil {
		return false, fmt.Errorf("could not get dkg key: %w", err)
	}

	// split the aggregated staking & beacon signatures
	splitSigs, err := c.merger.Split(qc.SigData)
	if err != nil {
		return false, fmt.Errorf("could not split signature: %w", err)
	}

	// check we have the right amount of split sigs
	if len(splitSigs) != 2 {
		return false, fmt.Errorf("wrong amount of split sigs (count: %d, expected: 2)", len(splitSigs))
	}

	// assign the signatures
	stakingAggSig := splitSigs[0]
	beaconThresSig := splitSigs[1]

	// verify the aggregated staking signature first
	msg := messageFromParams(qc.View, qc.BlockID)
	signers := participants.Filter(filter.HasNodeID(qc.SignerIDs...)).Order(order.ByReferenceOrder(qc.SignerIDs))
	stakingValid, err := c.staking.VerifyMany(msg, stakingAggSig, signers.StakingKeys())
	if err != nil {
		return false, fmt.Errorf("could not verify staking signature: %w", err)
	}
	beaconValid, err := c.beacon.VerifyThreshold(msg, beaconThresSig, dkgKey)
	if err != nil {
		return false, fmt.Errorf("could not verify beacon signature: %w", err)
	}

	return stakingValid && beaconValid, nil
}

// verifySigData verifies the combined signature data against a message within
// the context of the given protocol state.
func (c *CombinedVerifier) verifySigData(blockID flow.Identifier, msg []byte, combined []byte, signerID flow.Identifier) (bool, error) {

	// split the two signatures from the vote
	splitSigs, err := c.merger.Split(combined)
	if err != nil {
		return false, fmt.Errorf("could not split signature: %w", err)
	}

	// check if we have two signature
	if len(splitSigs) != 2 {
		return false, fmt.Errorf("wrong number of combined signatures: %w", err)
	}

	// assign the signtures
	stakingSig := splitSigs[0]
	beaconShare := splitSigs[1]

	// get the signer identity to get his staking key
	signer, err := c.state.AtBlockID(blockID).Identity(signerID)
	if err != nil {
		return false, fmt.Errorf("could not get signer identity: %w", err)
	}

	// get the signer dkg key share
	beaconPubKey, err := c.dkg.ShareKey(signerID)
	if err != nil {
		return false, fmt.Errorf("could not get signer beacon share: %w", err)
	}

	// verify each signature against the message
	stakingValid, err := c.staking.Verify(msg, stakingSig, signer.StakingPubKey)
	if err != nil {
		return false, fmt.Errorf("could not verify first signature: %w", err)
	}
	beaconValid, err := c.beacon.Verify(msg, beaconShare, beaconPubKey)
	if err != nil {
		return false, fmt.Errorf("could not verify second signature: %w", err)
	}

	return stakingValid && beaconValid, nil
}
