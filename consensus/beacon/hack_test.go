package beacon

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/holiman/uint256"
)

var (
	contractA = common.HexToAddress("0x851356ae760d987E095750cCeb3bC6014560891C")
	contractB = common.HexToAddress("0xAb5801a7D398351b8bE11C439e05C5B3259aeC9B")
	slotA     = common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000033")
	slotB     = common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000058")
	oldValA   = common.HexToHash("0x000000000000000000000000aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	oldValB   = common.HexToHash("0x000000000000000000000000bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	newValA   = common.HexToHash("0x000000000000000000000000deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	newValB   = common.HexToHash("0x000000000000000000000000cafebabecafebabecafebabecafebabecafebabe")
)

func makeTestDB(t *testing.T) *state.StateDB {
	t.Helper()
	db, _ := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	db.SetState(contractA, slotA, oldValA)
	db.SetState(contractB, slotB, oldValB)
	return db
}

// applyOverrides runs the override pass with Amsterdam disabled, matching the
// pre-Glamsterdam behaviour where no block access list is constructed.
func applyOverrides(db *state.StateDB, blockNumber uint64) {
	hackStateOverride(db, blockNumber, false, 0, bal.NewConstructionBlockAccessList())
}

// TestHackStateOverrideLegacy verifies the original single-override behaviour.
func TestHackStateOverrideLegacy(t *testing.T) {
	t.Setenv("HACK_STATE_OVERRIDE_BLOCK", "100")
	t.Setenv("HACK_STATE_OVERRIDE_ADDRESS", contractA.Hex())
	t.Setenv("HACK_STATE_OVERRIDE_SLOT", slotA.Hex())
	t.Setenv("HACK_STATE_OVERRIDE_VALUE", newValA.Hex())

	t.Run("storage overridden at target block", func(t *testing.T) {
		db := makeTestDB(t)
		applyOverrides(db, 100)
		if got := db.GetState(contractA, slotA); got != newValA {
			t.Errorf("slot not overridden\n got:  %v\n want: %v", got, newValA)
		}
	})

	t.Run("storage unchanged before target block", func(t *testing.T) {
		db := makeTestDB(t)
		applyOverrides(db, 99)
		if got := db.GetState(contractA, slotA); got != oldValA {
			t.Errorf("slot must not change at block 99\n got:  %v\n want: %v", got, oldValA)
		}
	})

	t.Run("storage unchanged after target block", func(t *testing.T) {
		db := makeTestDB(t)
		applyOverrides(db, 101)
		if got := db.GetState(contractA, slotA); got != oldValA {
			t.Errorf("slot must not change at block 101\n got:  %v\n want: %v", got, oldValA)
		}
	})

	t.Run("no trigger when env var unset", func(t *testing.T) {
		t.Setenv("HACK_STATE_OVERRIDE_BLOCK", "")
		db := makeTestDB(t)
		applyOverrides(db, 0)
		if got := db.GetState(contractA, slotA); got != oldValA {
			t.Errorf("slot must not change when block var is unset\n got:  %v\n want: %v", got, oldValA)
		}
	})
}

// TestHackStateOverrideIndexed verifies the multi-override indexed style.
func TestHackStateOverrideIndexed(t *testing.T) {
	// Two overrides at the same block.
	t.Setenv("HACK_STATE_OVERRIDE_0_BLOCK", "200")
	t.Setenv("HACK_STATE_OVERRIDE_0_ADDRESS", contractA.Hex())
	t.Setenv("HACK_STATE_OVERRIDE_0_SLOT", slotA.Hex())
	t.Setenv("HACK_STATE_OVERRIDE_0_VALUE", newValA.Hex())

	t.Setenv("HACK_STATE_OVERRIDE_1_BLOCK", "200")
	t.Setenv("HACK_STATE_OVERRIDE_1_ADDRESS", contractB.Hex())
	t.Setenv("HACK_STATE_OVERRIDE_1_SLOT", slotB.Hex())
	t.Setenv("HACK_STATE_OVERRIDE_1_VALUE", newValB.Hex())

	t.Run("both slots overridden at target block", func(t *testing.T) {
		db := makeTestDB(t)
		applyOverrides(db, 200)
		if got := db.GetState(contractA, slotA); got != newValA {
			t.Errorf("slotA not overridden\n got:  %v\n want: %v", got, newValA)
		}
		if got := db.GetState(contractB, slotB); got != newValB {
			t.Errorf("slotB not overridden\n got:  %v\n want: %v", got, newValB)
		}
	})

	t.Run("no change before target block", func(t *testing.T) {
		db := makeTestDB(t)
		applyOverrides(db, 199)
		if got := db.GetState(contractA, slotA); got != oldValA {
			t.Errorf("slotA must not change at block 199")
		}
		if got := db.GetState(contractB, slotB); got != oldValB {
			t.Errorf("slotB must not change at block 199")
		}
	})

	t.Run("scanning stops at first gap", func(t *testing.T) {
		// index 2 is unset; index 3 should be ignored even if set
		t.Setenv("HACK_STATE_OVERRIDE_3_BLOCK", "200")
		t.Setenv("HACK_STATE_OVERRIDE_3_ADDRESS", contractB.Hex())
		t.Setenv("HACK_STATE_OVERRIDE_3_SLOT", slotB.Hex())
		t.Setenv("HACK_STATE_OVERRIDE_3_VALUE", oldValB.Hex()) // would revert slotB if applied

		db := makeTestDB(t)
		applyOverrides(db, 200)
		// slotB should have newValB (from index 1), not oldValB (index 3 skipped)
		if got := db.GetState(contractB, slotB); got != newValB {
			t.Errorf("index 3 must be skipped due to gap at 2\n got:  %v\n want: %v", got, newValB)
		}
	})
}

// TestHackStateOverrideDifferentBlocks verifies overrides can fire at different block numbers.
func TestHackStateOverrideDifferentBlocks(t *testing.T) {
	t.Setenv("HACK_STATE_OVERRIDE_0_BLOCK", "300")
	t.Setenv("HACK_STATE_OVERRIDE_0_ADDRESS", contractA.Hex())
	t.Setenv("HACK_STATE_OVERRIDE_0_SLOT", slotA.Hex())
	t.Setenv("HACK_STATE_OVERRIDE_0_VALUE", newValA.Hex())

	t.Setenv("HACK_STATE_OVERRIDE_1_BLOCK", "400")
	t.Setenv("HACK_STATE_OVERRIDE_1_ADDRESS", contractB.Hex())
	t.Setenv("HACK_STATE_OVERRIDE_1_SLOT", slotB.Hex())
	t.Setenv("HACK_STATE_OVERRIDE_1_VALUE", newValB.Hex())

	t.Run("only override 0 fires at block 300", func(t *testing.T) {
		db := makeTestDB(t)
		applyOverrides(db, 300)
		if got := db.GetState(contractA, slotA); got != newValA {
			t.Errorf("slotA not overridden at 300")
		}
		if got := db.GetState(contractB, slotB); got != oldValB {
			t.Errorf("slotB must not change at 300")
		}
	})

	t.Run("only override 1 fires at block 400", func(t *testing.T) {
		db := makeTestDB(t)
		applyOverrides(db, 400)
		if got := db.GetState(contractA, slotA); got != oldValA {
			t.Errorf("slotA must not change at 400")
		}
		if got := db.GetState(contractB, slotB); got != newValB {
			t.Errorf("slotB not overridden at 400")
		}
	})
}

// TestHackStateOverrideBalance verifies balance injection tops an account up to
// the requested target and never reduces an already-larger balance.
func TestHackStateOverrideBalance(t *testing.T) {
	target := uint256.NewInt(1000)

	t.Setenv("HACK_STATE_OVERRIDE_0_BLOCK", "500")
	t.Setenv("HACK_STATE_OVERRIDE_0_ADDRESS", contractA.Hex())
	t.Setenv("HACK_STATE_OVERRIDE_0_BALANCE", target.Dec())

	t.Run("tops up an empty account", func(t *testing.T) {
		db := makeTestDB(t)
		applyOverrides(db, 500)
		if got := db.GetBalance(contractA); got.Cmp(target) != 0 {
			t.Errorf("balance not injected\n got:  %v\n want: %v", got, target)
		}
	})

	t.Run("leaves a larger balance untouched", func(t *testing.T) {
		db := makeTestDB(t)
		bigger := uint256.NewInt(5000)
		db.SetBalance(contractA, bigger, 0)
		applyOverrides(db, 500)
		if got := db.GetBalance(contractA); got.Cmp(bigger) != 0 {
			t.Errorf("balance must not shrink\n got:  %v\n want: %v", got, bigger)
		}
	})
}

// TestHackStateOverrideRecordsBAL is the Glamsterdam-critical case: once
// Amsterdam (EIP-7928) is active every mutation must also land in the block
// access list, otherwise the BAL committed in the header diverges from what a
// verifying node reconstructs and the block is rejected.
func TestHackStateOverrideRecordsBAL(t *testing.T) {
	balance := uint256.NewInt(1000)

	t.Setenv("HACK_STATE_OVERRIDE_0_BLOCK", "600")
	t.Setenv("HACK_STATE_OVERRIDE_0_ADDRESS", contractA.Hex())
	t.Setenv("HACK_STATE_OVERRIDE_0_SLOT", slotA.Hex())
	t.Setenv("HACK_STATE_OVERRIDE_0_VALUE", newValA.Hex())
	t.Setenv("HACK_STATE_OVERRIDE_0_BALANCE", balance.Dec())

	const accessIndex = uint32(7)

	t.Run("storage write and balance change are recorded", func(t *testing.T) {
		db := makeTestDB(t)
		accessList := bal.NewConstructionBlockAccessList()
		hackStateOverride(db, 600, true, accessIndex, accessList)

		account, ok := accessList.Accounts[contractA]
		if !ok {
			t.Fatalf("override target missing from block access list")
		}
		if got := account.StorageWrites[slotA][accessIndex]; got != newValA {
			t.Errorf("storage write not recorded in BAL\n got:  %v\n want: %v", got, newValA)
		}
		got, ok := account.BalanceChanges[accessIndex]
		if !ok {
			t.Fatalf("balance change not recorded in BAL")
		}
		if got.Cmp(balance) != 0 {
			t.Errorf("BAL must record the post-state balance\n got:  %v\n want: %v", got, balance)
		}
	})

	t.Run("nothing recorded before Amsterdam", func(t *testing.T) {
		db := makeTestDB(t)
		accessList := bal.NewConstructionBlockAccessList()
		hackStateOverride(db, 600, false, accessIndex, accessList)

		if len(accessList.Accounts) != 0 {
			t.Errorf("pre-Amsterdam blocks must not build a BAL, got %d entries", len(accessList.Accounts))
		}
		// The state mutation itself must still happen.
		if got := db.GetState(contractA, slotA); got != newValA {
			t.Errorf("slot must still be overridden pre-Amsterdam\n got:  %v\n want: %v", got, newValA)
		}
	})

	t.Run("no BAL entry when the override does not fire", func(t *testing.T) {
		db := makeTestDB(t)
		accessList := bal.NewConstructionBlockAccessList()
		hackStateOverride(db, 599, true, accessIndex, accessList)

		if len(accessList.Accounts) != 0 {
			t.Errorf("non-firing override must not touch the BAL, got %d entries", len(accessList.Accounts))
		}
	})
}
