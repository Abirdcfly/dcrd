// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"encoding/binary"
	"fmt"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1"
	"github.com/decred/dcrd/dcrutil"
)

const (
	// MaxDataCarrierSize is the maximum number of bytes allowed in pushed
	// data to be considered a nulldata transaction.
	MaxDataCarrierSize = 256

	// nilAddrErrStr is the common error string to use for attempts to
	// generate payment scripts to nil addresses embedded within a
	// dcrutil.Address interface.
	nilAddrErrStr = "unable to generate payment script for nil address"
)

// ScriptClass is an enumeration for the list of standard types of script.
type ScriptClass byte

// Classes of script payment known about in the blockchain.
const (
	NonStandardTy     ScriptClass = iota // None of the recognized forms.
	PubKeyTy                             // Pay pubkey.
	PubKeyHashTy                         // Pay pubkey hash.
	ScriptHashTy                         // Pay to script hash.
	MultiSigTy                           // Multi signature.
	NullDataTy                           // Empty data-only (provably prunable).
	StakeSubmissionTy                    // Stake submission.
	StakeGenTy                           // Stake generation
	StakeRevocationTy                    // Stake revocation.
	StakeSubChangeTy                     // Change for stake submission tx.
	PubkeyAltTy                          // Alternative signature pubkey.
	PubkeyHashAltTy                      // Alternative signature pubkey hash.
)

// scriptClassToName houses the human-readable strings which describe each
// script class.
var scriptClassToName = []string{
	NonStandardTy:     "nonstandard",
	PubKeyTy:          "pubkey",
	PubkeyAltTy:       "pubkeyalt",
	PubKeyHashTy:      "pubkeyhash",
	PubkeyHashAltTy:   "pubkeyhashalt",
	ScriptHashTy:      "scripthash",
	MultiSigTy:        "multisig",
	NullDataTy:        "nulldata",
	StakeSubmissionTy: "stakesubmission",
	StakeGenTy:        "stakegen",
	StakeRevocationTy: "stakerevoke",
	StakeSubChangeTy:  "sstxchange",
}

// String implements the Stringer interface by returning the name of
// the enum script class. If the enum is invalid then "Invalid" will be
// returned.
func (t ScriptClass) String() string {
	if int(t) > len(scriptClassToName) || int(t) < 0 {
		return "Invalid"
	}
	return scriptClassToName[t]
}

// isOneByteMaxDataPush returns true if the parsed opcode pushes exactly one
// byte to the stack.
func isOneByteMaxDataPush(po parsedOpcode) bool {
	return po.opcode.value == OP_1 ||
		po.opcode.value == OP_2 ||
		po.opcode.value == OP_3 ||
		po.opcode.value == OP_4 ||
		po.opcode.value == OP_5 ||
		po.opcode.value == OP_6 ||
		po.opcode.value == OP_7 ||
		po.opcode.value == OP_8 ||
		po.opcode.value == OP_9 ||
		po.opcode.value == OP_10 ||
		po.opcode.value == OP_11 ||
		po.opcode.value == OP_12 ||
		po.opcode.value == OP_13 ||
		po.opcode.value == OP_14 ||
		po.opcode.value == OP_15 ||
		po.opcode.value == OP_16 ||
		po.opcode.value == OP_DATA_1
}

// isPubkey returns true if the script passed is an alternative pay-to-pubkey
// transaction, false otherwise.
func isPubkeyAlt(pops []parsedOpcode) bool {
	// An alternative pubkey must be less than 512 bytes.
	return len(pops) == 3 &&
		len(pops[0].data) < 512 &&
		isOneByteMaxDataPush(pops[1]) &&
		pops[2].opcode.value == OP_CHECKSIGALT
}

// isPubkeyHashAlt returns true if the script passed is a pay-to-pubkey-hash
// transaction, false otherwise.
func isPubkeyHashAlt(pops []parsedOpcode) bool {
	return len(pops) == 6 &&
		pops[0].opcode.value == OP_DUP &&
		pops[1].opcode.value == OP_HASH160 &&
		pops[2].opcode.value == OP_DATA_20 &&
		pops[3].opcode.value == OP_EQUALVERIFY &&
		isOneByteMaxDataPush(pops[4]) &&
		pops[5].opcode.value == OP_CHECKSIGALT
}

// multiSigDetails houses details extracted from a standard multisig script.
type multiSigDetails struct {
	requiredSigs int
	numPubKeys   int
	pubKeys      [][]byte
	valid        bool
}

// extractMultisigScriptDetails attempts to extract details from the passed
// script if it is a standard multisig script.  The returned details struct will
// have the valid flag set to false otherwise.
//
// The extract pubkeys flag indicates whether or not the pubkeys themselves
// should also be extracted and is provided because extracting them results in
// an allocation that the caller might wish to avoid.  The pubKeys member of
// the returned details struct will be nil when the flag is false.
//
// NOTE: This function is only valid for version 0 scripts.  The returned
// details struct will always be empty and have the valid flag set to false for
// other script versions.
func extractMultisigScriptDetails(scriptVersion uint16, script []byte, extractPubKeys bool) multiSigDetails {
	// The only currently supported script version is 0.
	if scriptVersion != 0 {
		return multiSigDetails{}
	}

	// A multi-signature script is of the form:
	//  NUM_SIGS PUBKEY PUBKEY PUBKEY ... NUM_PUBKEYS OP_CHECKMULTISIG

	// The script can't possibly be a multisig script if it doesn't end with
	// OP_CHECKMULTISIG or have at least two small integer pushes preceding it.
	// Fail fast to avoid more work below.
	if len(script) < 3 || script[len(script)-1] != OP_CHECKMULTISIG {
		return multiSigDetails{}
	}

	// The first opcode must be a small integer specifying the number of
	// signatures required.
	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	if !tokenizer.Next() || !isSmallInt(tokenizer.Opcode()) {
		return multiSigDetails{}
	}
	requiredSigs := asSmallInt(tokenizer.Opcode())

	// The next series of opcodes must either push public keys or be a small
	// integer specifying the number of public keys.
	var numPubKeys int
	var pubKeys [][]byte
	if extractPubKeys {
		pubKeys = make([][]byte, 0, MaxPubKeysPerMultiSig)
	}
	for tokenizer.Next() {
		data := tokenizer.Data()
		if !isStrictPubKeyEncoding(data) {
			break
		}
		numPubKeys++
		if extractPubKeys {
			pubKeys = append(pubKeys, data)
		}
	}
	if tokenizer.Done() {
		return multiSigDetails{}
	}

	// The next opcode must be a small integer specifying the number of public
	// keys required.
	op := tokenizer.Opcode()
	if !isSmallInt(op) || asSmallInt(op) != numPubKeys {
		return multiSigDetails{}
	}

	// There must only be a single opcode left unparsed which will be
	// OP_CHECKMULTISIG per the check above.
	if int32(len(tokenizer.Script()))-tokenizer.ByteIndex() != 1 {
		return multiSigDetails{}
	}

	return multiSigDetails{
		requiredSigs: requiredSigs,
		numPubKeys:   numPubKeys,
		pubKeys:      pubKeys,
		valid:        true,
	}
}

// isMultisigScript returns whether or not the passed script is a standard
// multisig script.
//
// NOTE: This function is only valid for version 0 scripts.  It will always
// return false for other script versions.
func isMultisigScript(scriptVersion uint16, script []byte) bool {
	// Since this is only checking the form of the script, don't extract the
	// public keys to avoid the allocation.
	details := extractMultisigScriptDetails(scriptVersion, script, false)
	return details.valid
}

// IsMultisigScript returns whether or not the passed script is a standard
// multisignature script.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
//
// The error is DEPRECATED and will be removed in the major version bump.
func IsMultisigScript(script []byte) (bool, error) {
	const scriptVersion = 0
	return isMultisigScript(scriptVersion, script), nil
}

// IsMultisigSigScript returns whether or not the passed script appears to be a
// signature script which consists of a pay-to-script-hash multi-signature
// redeem script.  Determining if a signature script is actually a redemption of
// pay-to-script-hash requires the associated public key script which is often
// expensive to obtain.  Therefore, this makes a fast best effort guess that has
// a high probability of being correct by checking if the signature script ends
// with a data push and treating that data push as if it were a p2sh redeem
// script
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func IsMultisigSigScript(script []byte) bool {
	const scriptVersion = 0

	// The script can't possibly be a multisig signature script if it doesn't
	// end with OP_CHECKMULTISIG in the redeem script or have at least two small
	// integers preceding it, and the redeem script itself must be preceded by
	// at least a data push opcode.  Fail fast to avoid more work below.
	if len(script) < 4 || script[len(script)-1] != OP_CHECKMULTISIG {
		return false
	}

	// Parse through the script to find the last opcode and any data it might
	// push and treat it as a p2sh redeem script even though it might not
	// actually be one.
	possibleRedeemScript := finalOpcodeData(scriptVersion, script)
	if possibleRedeemScript == nil {
		return false
	}

	// Finally, return if that possible redeem script is a multisig script.
	return isMultisigScript(scriptVersion, possibleRedeemScript)
}

// extractCompressedPubKey extracts a compressed public key from the passed
// script if it is a standard pay-to-compressed-secp256k1-pubkey script.  It
// will return nil otherwise.
func extractCompressedPubKey(script []byte) []byte {
	// A pay-to-compressed-pubkey script is of the form:
	//  OP_DATA_33 <33-byte compresed pubkey> OP_CHECKSIG

	// All compressed secp256k1 public keys must start with 0x02 or 0x03.
	if len(script) == 35 &&
		script[34] == OP_CHECKSIG &&
		script[0] == OP_DATA_33 &&
		(script[1] == 0x02 || script[1] == 0x03) {

		return script[1:34]
	}
	return nil
}

// extractUncompressedPubKey extracts an uncompressed public key from the
// passed script if it is a standard pay-to-uncompressed-secp256k1-pubkey
// script.  It will return nil otherwise.
func extractUncompressedPubKey(script []byte) []byte {
	// A pay-to-compressed-pubkey script is of the form:
	//  OP_DATA_65 <65-byte uncompressed pubkey> OP_CHECKSIG

	// All non-hybrid uncompressed secp256k1 public keys must start with 0x04.
	if len(script) == 67 &&
		script[66] == OP_CHECKSIG &&
		script[0] == OP_DATA_65 &&
		script[1] == 0x04 {

		return script[1:66]
	}
	return nil
}

// extractPubKey extracts either compressed or uncompressed public key from the
// passed script if it is a either a standard pay-to-compressed-secp256k1-pubkey
// or pay-to-uncompressed-secp256k1-pubkey script, respectively.  It will return
// nil otherwise.
func extractPubKey(script []byte) []byte {
	if pubKey := extractCompressedPubKey(script); pubKey != nil {
		return pubKey
	}
	return extractUncompressedPubKey(script)
}

// isPubKeyScript returns whether or not the passed script is either a standard
// pay-to-compressed-secp256k1-pubkey or pay-to-uncompressed-secp256k1-pubkey
// script.
func isPubKeyScript(script []byte) bool {
	return extractPubKey(script) != nil
}

// extractPubKeyAltDetails extracts the public key and signature type from the
// passed script if it is a standard pay-to-alt-pubkey script.  It will return
// nil otherwise.
func extractPubKeyAltDetails(script []byte) ([]byte, dcrec.SignatureType) {
	// A pay-to-alt-pubkey script is of the form:
	//  PUBKEY SIGTYPE OP_CHECKSIGALT
	//
	// The only two currently supported alternative signature types are ed25519
	// and schnorr + secp256k1 (with a compressed pubkey).
	//
	//  OP_DATA_32 <32-byte pubkey> <1-byte ed25519 sigtype> OP_CHECKSIGALT
	//  OP_DATA_33 <33-byte pubkey> <1-byte schnorr+secp sigtype> OP_CHECKSIGALT

	// The script can't possibly be a a pay-to-alt-pubkey script if it doesn't
	// end with OP_CHECKSIGALT or have at least two small integer pushes
	// preceding it (although any reasonable pubkey will certainly be larger).
	// Fail fast to avoid more work below.
	if len(script) < 3 || script[len(script)-1] != OP_CHECKSIGALT {
		return nil, 0
	}

	if len(script) == 35 && script[0] == OP_DATA_32 &&
		isSmallInt(script[33]) && asSmallInt(script[33]) == dcrec.STEd25519 {

		return script[1:33], dcrec.STEd25519
	}

	if len(script) == 36 && script[0] == OP_DATA_33 &&
		isSmallInt(script[34]) &&
		asSmallInt(script[34]) == dcrec.STSchnorrSecp256k1 &&
		isStrictPubKeyEncoding(script[1:34]) {

		return script[1:34], dcrec.STSchnorrSecp256k1
	}

	return nil, 0
}

// isPubKeyAltScript returns whether or not the passed script is a standard
// pay-to-alt-pubkey script.
func isPubKeyAltScript(script []byte) bool {
	pk, _ := extractPubKeyAltDetails(script)
	return pk != nil
}

// extractPubKeyHash extracts the public key hash from the passed script if it
// is a standard pay-to-pubkey-hash script.  It will return nil otherwise.
func extractPubKeyHash(script []byte) []byte {
	// A pay-to-pubkey-hash script is of the form:
	//  OP_DUP OP_HASH160 <20-byte hash> OP_EQUALVERIFY OP_CHECKSIG
	if len(script) == 25 &&
		script[0] == OP_DUP &&
		script[1] == OP_HASH160 &&
		script[2] == OP_DATA_20 &&
		script[23] == OP_EQUALVERIFY &&
		script[24] == OP_CHECKSIG {

		return script[3:23]
	}

	return nil
}

// isPubKeyHashScript returns whether or not the passed script is a standard
// pay-to-pubkey-hash script.
func isPubKeyHashScript(script []byte) bool {
	return extractPubKeyHash(script) != nil
}

// isStandardAltSignatureType returns whether or not the provided opcode
// represents a push of a standard alt signature type.
func isStandardAltSignatureType(op byte) bool {
	if !isSmallInt(op) {
		return false
	}

	sigType := asSmallInt(op)
	return sigType == dcrec.STEd25519 || sigType == dcrec.STSchnorrSecp256k1
}

// extractPubKeyHashAltDetails extracts the public key hash and signature type
// from the passed script if it is a standard pay-to-alt-pubkey-hash script.  It
// will return nil otherwise.
func extractPubKeyHashAltDetails(script []byte) ([]byte, dcrec.SignatureType) {
	// A pay-to-alt-pubkey-hash script is of the form:
	//  DUP HASH160 <20-byte hash> EQUALVERIFY SIGTYPE CHECKSIG
	//
	// The only two currently supported alternative signature types are ed25519
	// and schnorr + secp256k1 (with a compressed pubkey).
	//
	//  DUP HASH160 <20-byte hash> EQUALVERIFY <1-byte ed25519 sigtype> CHECKSIG
	//  DUP HASH160 <20-byte hash> EQUALVERIFY <1-byte schnorr+secp sigtype> CHECKSIG
	//
	//  Notice that OP_0 is not specified since signature type 0 disabled.

	if len(script) == 26 &&
		script[0] == OP_DUP &&
		script[1] == OP_HASH160 &&
		script[2] == OP_DATA_20 &&
		script[23] == OP_EQUALVERIFY &&
		isStandardAltSignatureType(script[24]) &&
		script[25] == OP_CHECKSIGALT {

		return script[3:23], dcrec.SignatureType(asSmallInt(script[24]))
	}

	return nil, 0
}

// isPubKeyHashAltScript returns whether or not the passed script is a standard
// pay-to-alt-pubkey-hash script.
func isPubKeyHashAltScript(script []byte) bool {
	pk, _ := extractPubKeyHashAltDetails(script)
	return pk != nil
}

// isNullDataScript returns whether or not the passed script is a standard
// null data script.
//
// NOTE: This function is only valid for version 0 scripts.  It will always
// return false for other script versions.
func isNullDataScript(scriptVersion uint16, script []byte) bool {
	// The only currently supported script version is 0.
	if scriptVersion != 0 {
		return false
	}

	// A null script is of the form:
	//  OP_RETURN <optional data>
	//
	// Thus, it can either be a single OP_RETURN or an OP_RETURN followed by a
	// data push up to MaxDataCarrierSize bytes.

	// The script can't possibly be a a null data script if it doesn't start
	// with OP_RETURN.  Fail fast to avoid more work below.
	if len(script) < 1 || script[0] != OP_RETURN {
		return false
	}

	// Single OP_RETURN.
	if len(script) == 1 {
		return true
	}

	// OP_RETURN followed by data push up to MaxDataCarrierSize bytes.
	tokenizer := MakeScriptTokenizer(scriptVersion, script[1:])
	return tokenizer.Next() && tokenizer.Done() &&
		(isSmallInt(tokenizer.Opcode()) || tokenizer.Opcode() <= OP_PUSHDATA4) &&
		len(tokenizer.Data()) <= MaxDataCarrierSize
}

// extractStakePubKeyHash extracts the public key hash from the passed script if
// it is a standard stake-tagged pay-to-pubkey-hash script with the provided
// stake opcode.  It will return nil otherwise.
func extractStakePubKeyHash(script []byte, stakeOpcode byte) []byte {
	// A stake-tagged pay-to-pubkey-hash is of the form:
	//   <stake opcode> <standard-pay-to-pubkey-hash script>

	// The script can't possibly be a stake-tagged pay-to-pubkey-hash if it
	// doesn't start with the given stake opcode.  Fail fast to avoid more work
	// below.
	if len(script) < 1 || script[0] != stakeOpcode {
		return nil
	}

	return extractPubKeyHash(script[1:])
}

// extractStakeScriptHash extracts the script hash from the passed script if it
// is a standard stake-tagged pay-to-script-hash script with the provided stake
// opcode.  It will return nil otherwise.
func extractStakeScriptHash(script []byte, stakeOpcode byte) []byte {
	// A stake-tagged pay-to-script-hash is of the form:
	//   <stake opcode> <standard-pay-to-script-hash script>

	// The script can't possibly be a stake-tagged pay-to-script-hash if it
	// doesn't start with the given stake opcode.  Fail fast to avoid more work
	// below.
	if len(script) < 1 || script[0] != stakeOpcode {
		return nil
	}

	return extractScriptHash(script[1:])
}

// isStakeSubmissionScript returns whether or not the passed script is a
// supported stake submission script.
//
// NOTE: This function is only valid for version 0 scripts.  It will always
// return false for other script versions.
func isStakeSubmissionScript(scriptVersion uint16, script []byte) bool {
	// The only currently supported script version is 0.
	if scriptVersion != 0 {
		return false
	}

	// The only supported stake submission scripts are pay-to-pubkey-hash and
	// pay-to-script-hash tagged with the stake submission opcode.
	const stakeOpcode = OP_SSTX
	return extractStakePubKeyHash(script, stakeOpcode) != nil ||
		extractStakeScriptHash(script, stakeOpcode) != nil
}

// isStakeGen returns true if the script passed is a stake generation tx,
// false otherwise.
func isStakeGen(pops []parsedOpcode) bool {
	if len(pops) == 6 &&
		pops[0].opcode.value == OP_SSGEN &&
		pops[1].opcode.value == OP_DUP &&
		pops[2].opcode.value == OP_HASH160 &&
		pops[3].opcode.value == OP_DATA_20 &&
		pops[4].opcode.value == OP_EQUALVERIFY &&
		pops[5].opcode.value == OP_CHECKSIG {
		return true
	}

	if len(pops) == 4 &&
		pops[0].opcode.value == OP_SSGEN &&
		pops[1].opcode.value == OP_HASH160 &&
		pops[2].opcode.value == OP_DATA_20 &&
		pops[3].opcode.value == OP_EQUAL {
		return true
	}

	return false
}

// isStakeRevocation returns true if the script passed is a stake submission
// revocation tx, false otherwise.
func isStakeRevocation(pops []parsedOpcode) bool {
	if len(pops) == 6 &&
		pops[0].opcode.value == OP_SSRTX &&
		pops[1].opcode.value == OP_DUP &&
		pops[2].opcode.value == OP_HASH160 &&
		pops[3].opcode.value == OP_DATA_20 &&
		pops[4].opcode.value == OP_EQUALVERIFY &&
		pops[5].opcode.value == OP_CHECKSIG {
		return true
	}

	if len(pops) == 4 &&
		pops[0].opcode.value == OP_SSRTX &&
		pops[1].opcode.value == OP_HASH160 &&
		pops[2].opcode.value == OP_DATA_20 &&
		pops[3].opcode.value == OP_EQUAL {
		return true
	}

	return false
}

// isSStxChange returns true if the script passed is a stake submission
// change tx, false otherwise.
func isSStxChange(pops []parsedOpcode) bool {
	if len(pops) == 6 &&
		pops[0].opcode.value == OP_SSTXCHANGE &&
		pops[1].opcode.value == OP_DUP &&
		pops[2].opcode.value == OP_HASH160 &&
		pops[3].opcode.value == OP_DATA_20 &&
		pops[4].opcode.value == OP_EQUALVERIFY &&
		pops[5].opcode.value == OP_CHECKSIG {
		return true
	}

	if len(pops) == 4 &&
		pops[0].opcode.value == OP_SSTXCHANGE &&
		pops[1].opcode.value == OP_HASH160 &&
		pops[2].opcode.value == OP_DATA_20 &&
		pops[3].opcode.value == OP_EQUAL {
		return true
	}

	return false
}

// scriptType returns the type of the script being inspected from the known
// standard types.
//
// NOTE:  All scripts that are not version 0 are currently considered non
// standard.
func typeOfScript(scriptVersion uint16, script []byte) ScriptClass {
	if scriptVersion != 0 {
		return NonStandardTy
	}

	switch {
	case isPubKeyScript(script):
		return PubKeyTy
	case isPubKeyAltScript(script):
		return PubkeyAltTy
	case isPubKeyHashScript(script):
		return PubKeyHashTy
	case isPubKeyHashAltScript(script):
		return PubkeyHashAltTy
	case isScriptHashScript(script):
		return ScriptHashTy
	case isMultisigScript(scriptVersion, script):
		return MultiSigTy
	case isNullDataScript(scriptVersion, script):
		return NullDataTy
	case isStakeSubmissionScript(scriptVersion, script):
		return StakeSubmissionTy
	}

	pops, err := parseScript(script)
	if err != nil {
		return NonStandardTy
	}

	switch {
	case isStakeGen(pops):
		return StakeGenTy
	case isStakeRevocation(pops):
		return StakeRevocationTy
	case isSStxChange(pops):
		return StakeSubChangeTy
	}

	return NonStandardTy
}

// GetScriptClass returns the class of the script passed.
//
// NonStandardTy will be returned when the script does not parse.
func GetScriptClass(version uint16, script []byte) ScriptClass {
	if version != DefaultScriptVersion {
		return NonStandardTy
	}

	return typeOfScript(version, script)
}

// expectedInputs returns the number of arguments required by a script.
// If the script is of unknown type such that the number can not be determined
// then -1 is returned. We are an internal function and thus assume that class
// is the real class of pops (and we can thus assume things that were determined
// while finding out the type).
func expectedInputs(pops []parsedOpcode, class ScriptClass,
	subclass ScriptClass) int {
	switch class {
	case PubKeyTy:
		return 1

	case PubKeyHashTy:
		return 2

	case StakeSubmissionTy:
		if subclass == PubKeyHashTy {
			return 2
		}
		return 1 // P2SH

	case StakeGenTy:
		if subclass == PubKeyHashTy {
			return 2
		}
		return 1 // P2SH

	case StakeRevocationTy:
		if subclass == PubKeyHashTy {
			return 2
		}
		return 1 // P2SH

	case StakeSubChangeTy:
		if subclass == PubKeyHashTy {
			return 2
		}
		return 1 // P2SH

	case ScriptHashTy:
		// Not including script, handled below.
		return 1

	case MultiSigTy:
		// Standard multisig has a push a small number for the number
		// of sigs and number of keys.  Check the first push instruction
		// to see how many arguments are expected. typeOfScript already
		// checked this so we know it'll be a small int.  Also, due to
		// the original bitcoind bug where OP_CHECKMULTISIG pops an
		// additional item from the stack, add an extra expected input
		// for the extra push that is required to compensate.
		return asSmallInt(pops[0].opcode.value)

	case NullDataTy:
		fallthrough
	default:
		return -1
	}
}

// ScriptInfo houses information about a script pair that is determined by
// CalcScriptInfo.
type ScriptInfo struct {
	// PkScriptClass is the class of the public key script and is equivalent
	// to calling GetScriptClass on it.
	PkScriptClass ScriptClass

	// NumInputs is the number of inputs provided by the public key script.
	NumInputs int

	// ExpectedInputs is the number of outputs required by the signature
	// script and any pay-to-script-hash scripts. The number will be -1 if
	// unknown.
	ExpectedInputs int

	// SigOps is the number of signature operations in the script pair.
	SigOps int
}

// IsStakeOutput returns true is a script output is a stake type.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
//
// DEPRECATED.  This will be removed in the next major version bump.
func IsStakeOutput(pkScript []byte) bool {
	const scriptVersion = 0
	class := typeOfScript(scriptVersion, pkScript)
	return class == StakeSubmissionTy ||
		class == StakeGenTy ||
		class == StakeRevocationTy ||
		class == StakeSubChangeTy
}

// GetStakeOutSubclass extracts the subclass (P2PKH or P2SH)
// from a stake output.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func GetStakeOutSubclass(pkScript []byte) (ScriptClass, error) {
	const scriptVersion = 0
	if err := checkScriptParses(scriptVersion, pkScript); err != nil {
		return 0, err
	}

	class := typeOfScript(scriptVersion, pkScript)
	isStake := class == StakeSubmissionTy ||
		class == StakeGenTy ||
		class == StakeRevocationTy ||
		class == StakeSubChangeTy

	subClass := ScriptClass(0)
	if isStake {
		subClass = typeOfScript(scriptVersion, pkScript[1:])
	} else {
		return 0, fmt.Errorf("not a stake output")
	}

	return subClass, nil
}

// getStakeOutSubscript extracts the subscript (P2PKH or P2SH)
// from a stake output.
func getStakeOutSubscript(pkScript []byte) []byte {
	return pkScript[1:]
}

// ContainsStakeOpCodes returns whether or not a pkScript contains stake tagging
// OP codes.
func ContainsStakeOpCodes(pkScript []byte) (bool, error) {
	shPops, err := parseScript(pkScript)
	if err != nil {
		return false, err
	}

	for _, pop := range shPops {
		if isStakeOpcode(pop.opcode.value) {
			return true, nil
		}
	}

	return false, nil
}

// CalcScriptInfo returns a structure providing data about the provided script
// pair.  It will error if the pair is in someway invalid such that they can not
// be analysed, i.e. if they do not parse or the pkScript is not a push-only
// script
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
//
// DEPRECATED.  This will be removed in the next major version bump.
func CalcScriptInfo(sigScript, pkScript []byte, bip16 bool) (*ScriptInfo, error) {
	const scriptVersion = 0
	sigPops, err := parseScript(sigScript)
	if err != nil {
		return nil, err
	}

	pkPops, err := parseScript(pkScript)
	if err != nil {
		return nil, err
	}

	si := new(ScriptInfo)
	si.PkScriptClass = typeOfScript(scriptVersion, pkScript)

	// Can't have a signature script that doesn't just push data.
	if !isPushOnly(sigPops) {
		return nil, scriptError(ErrNotPushOnly,
			"signature script is not push only")
	}

	subClass := ScriptClass(0)
	if si.PkScriptClass == StakeSubmissionTy ||
		si.PkScriptClass == StakeGenTy ||
		si.PkScriptClass == StakeRevocationTy ||
		si.PkScriptClass == StakeSubChangeTy {
		subClass, err = GetStakeOutSubclass(pkScript)
		if err != nil {
			return nil, err
		}
	}

	si.ExpectedInputs = expectedInputs(pkPops, si.PkScriptClass, subClass)

	// All entries pushed to stack (or are OP_RESERVED and exec will fail).
	si.NumInputs = len(sigPops)

	// Count sigops taking into account pay-to-script-hash.
	if (si.PkScriptClass == ScriptHashTy || subClass == ScriptHashTy) && bip16 {
		// The pay-to-hash-script is the final data push of the
		// signature script.
		script := sigPops[len(sigPops)-1].data
		shPops, err := parseScript(script)
		if err != nil {
			return nil, err
		}

		reedeemClass := typeOfScript(scriptVersion, script)
		shInputs := expectedInputs(shPops, reedeemClass, 0)
		if shInputs == -1 {
			si.ExpectedInputs = -1
		} else {
			si.ExpectedInputs += shInputs
		}
		si.SigOps = getSigOpCount(shPops, true)
	} else {
		si.SigOps = getSigOpCount(pkPops, true)
	}

	return si, nil
}

// CalcMultiSigStats returns the number of public keys and signatures from
// a multi-signature transaction script.  The passed script MUST already be
// known to be a multi-signature script.
func CalcMultiSigStats(script []byte) (int, int, error) {
	pops, err := parseScript(script)
	if err != nil {
		return 0, 0, err
	}

	// A multi-signature script is of the pattern:
	//  NUM_SIGS PUBKEY PUBKEY PUBKEY... NUM_PUBKEYS OP_CHECKMULTISIG
	// Therefore the number of signatures is the oldest item on the stack
	// and the number of pubkeys is the 2nd to last.  Also, the absolute
	// minimum for a multi-signature script is 1 pubkey, so at least 4
	// items must be on the stack per:
	//  OP_1 PUBKEY OP_1 OP_CHECKMULTISIG
	if len(pops) < 4 {
		str := fmt.Sprintf("script %x is not a multisig script", script)
		return 0, 0, scriptError(ErrNotMultisigScript, str)
	}

	numSigs := asSmallInt(pops[0].opcode.value)
	numPubKeys := asSmallInt(pops[len(pops)-2].opcode.value)
	return numPubKeys, numSigs, nil
}

// MultisigRedeemScriptFromScriptSig attempts to extract a multi-
// signature redeem script from a P2SH-redeeming input. It returns
// nil if the signature script is not a multisignature script.
func MultisigRedeemScriptFromScriptSig(script []byte) ([]byte, error) {
	pops, err := parseScript(script)
	if err != nil {
		return nil, err
	}

	// The redeemScript is always the last item on the stack of
	// the script sig.
	return pops[len(pops)-1].data, nil
}

// payToPubKeyHashScript creates a new script to pay a transaction
// output to a 20-byte pubkey hash. It is expected that the input is a valid
// hash.
func payToPubKeyHashScript(pubKeyHash []byte) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_DUP).AddOp(OP_HASH160).
		AddData(pubKeyHash).AddOp(OP_EQUALVERIFY).AddOp(OP_CHECKSIG).
		Script()
}

// payToPubKeyHashEdwardsScript creates a new script to pay a transaction
// output to a 20-byte pubkey hash of an Edwards public key. It is expected
// that the input is a valid hash.
func payToPubKeyHashEdwardsScript(pubKeyHash []byte) ([]byte, error) {
	edwardsData := []byte{byte(dcrec.STEd25519)}
	return NewScriptBuilder().AddOp(OP_DUP).AddOp(OP_HASH160).
		AddData(pubKeyHash).AddOp(OP_EQUALVERIFY).AddData(edwardsData).
		AddOp(OP_CHECKSIGALT).Script()
}

// payToPubKeyHashSchnorrScript creates a new script to pay a transaction
// output to a 20-byte pubkey hash of a secp256k1 public key, but expecting
// a schnorr signature instead of a classic secp256k1 signature. It is
// expected that the input is a valid hash.
func payToPubKeyHashSchnorrScript(pubKeyHash []byte) ([]byte, error) {
	schnorrData := []byte{byte(dcrec.STSchnorrSecp256k1)}
	return NewScriptBuilder().AddOp(OP_DUP).AddOp(OP_HASH160).
		AddData(pubKeyHash).AddOp(OP_EQUALVERIFY).AddData(schnorrData).
		AddOp(OP_CHECKSIGALT).Script()
}

// payToScriptHashScript creates a new script to pay a transaction output to a
// script hash. It is expected that the input is a valid hash.
func payToScriptHashScript(scriptHash []byte) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_HASH160).AddData(scriptHash).
		AddOp(OP_EQUAL).Script()
}

// GetScriptHashFromP2SHScript extracts the script hash from a valid
// P2SH pkScript.
func GetScriptHashFromP2SHScript(pkScript []byte) ([]byte, error) {
	pops, err := parseScript(pkScript)
	if err != nil {
		return nil, err
	}

	var sh []byte
	reachedHash160DataPush := false
	for _, p := range pops {
		if p.opcode.value == OP_HASH160 {
			reachedHash160DataPush = true
			continue
		}
		if reachedHash160DataPush {
			sh = p.data
			break
		}
	}

	return sh, nil
}

// PayToScriptHashScript is the exported version of payToScriptHashScript.
func PayToScriptHashScript(scriptHash []byte) ([]byte, error) {
	return payToScriptHashScript(scriptHash)
}

// payToPubkeyScript creates a new script to pay a transaction output to a
// public key. It is expected that the input is a valid pubkey.
func payToPubKeyScript(serializedPubKey []byte) ([]byte, error) {
	return NewScriptBuilder().AddData(serializedPubKey).
		AddOp(OP_CHECKSIG).Script()
}

// payToEdwardsPubKeyScript creates a new script to pay a transaction output
// to an Ed25519 public key. It is expected that the input is a valid pubkey.
func payToEdwardsPubKeyScript(serializedPubKey []byte) ([]byte, error) {
	edwardsData := []byte{byte(dcrec.STEd25519)}
	return NewScriptBuilder().AddData(serializedPubKey).AddData(edwardsData).
		AddOp(OP_CHECKSIGALT).Script()
}

// payToSchnorrPubKeyScript creates a new script to pay a transaction output
// to a secp256k1 public key, but to be signed by Schnorr type signature. It
// is expected that the input is a valid pubkey.
func payToSchnorrPubKeyScript(serializedPubKey []byte) ([]byte, error) {
	schnorrData := []byte{byte(dcrec.STSchnorrSecp256k1)}
	return NewScriptBuilder().AddData(serializedPubKey).AddData(schnorrData).
		AddOp(OP_CHECKSIGALT).Script()
}

// PayToSStx creates a new script to pay a transaction output to a script hash or
// public key hash, but tags the output with OP_SSTX. For use in constructing
// valid SStxs.
func PayToSStx(addr dcrutil.Address) ([]byte, error) {
	// Only pay to pubkey hash and pay to script hash are
	// supported.
	scriptType := PubKeyHashTy
	switch addr := addr.(type) {
	case *dcrutil.AddressPubKeyHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		if addr.DSA(addr.Net()) != dcrec.STEcdsaSecp256k1 {
			str := "unable to generate payment script for " +
				"unsupported digital signature algorithm"
			return nil, scriptError(ErrUnsupportedAddress, str)
		}

	case *dcrutil.AddressScriptHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		scriptType = ScriptHashTy

	default:
		str := fmt.Sprintf("unable to generate payment script for "+
			"unsupported address type %T", addr)
		return nil, scriptError(ErrUnsupportedAddress, str)
	}

	hash := addr.ScriptAddress()

	if scriptType == PubKeyHashTy {
		return NewScriptBuilder().AddOp(OP_SSTX).AddOp(OP_DUP).
			AddOp(OP_HASH160).AddData(hash).AddOp(OP_EQUALVERIFY).
			AddOp(OP_CHECKSIG).Script()
	}
	return NewScriptBuilder().AddOp(OP_SSTX).AddOp(OP_HASH160).
		AddData(hash).AddOp(OP_EQUAL).Script()
}

// PayToSStxChange creates a new script to pay a transaction output to a
// public key hash, but tags the output with OP_SSTXCHANGE. For use in constructing
// valid SStxs.
func PayToSStxChange(addr dcrutil.Address) ([]byte, error) {
	// Only pay to pubkey hash and pay to script hash are
	// supported.
	scriptType := PubKeyHashTy
	switch addr := addr.(type) {
	case *dcrutil.AddressPubKeyHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		if addr.DSA(addr.Net()) != dcrec.STEcdsaSecp256k1 {
			str := "unable to generate payment script for " +
				"unsupported digital signature algorithm"
			return nil, scriptError(ErrUnsupportedAddress, str)
		}

	case *dcrutil.AddressScriptHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		scriptType = ScriptHashTy

	default:
		str := fmt.Sprintf("unable to generate payment script for "+
			"unsupported address type %T", addr)
		return nil, scriptError(ErrUnsupportedAddress, str)
	}

	hash := addr.ScriptAddress()

	if scriptType == PubKeyHashTy {
		return NewScriptBuilder().AddOp(OP_SSTXCHANGE).AddOp(OP_DUP).
			AddOp(OP_HASH160).AddData(hash).AddOp(OP_EQUALVERIFY).
			AddOp(OP_CHECKSIG).Script()
	}
	return NewScriptBuilder().AddOp(OP_SSTXCHANGE).AddOp(OP_HASH160).
		AddData(hash).AddOp(OP_EQUAL).Script()
}

// PayToSSGen creates a new script to pay a transaction output to a public key
// hash or script hash, but tags the output with OP_SSGEN. For use in constructing
// valid SSGen txs.
func PayToSSGen(addr dcrutil.Address) ([]byte, error) {
	// Only pay to pubkey hash and pay to script hash are
	// supported.
	scriptType := PubKeyHashTy
	switch addr := addr.(type) {
	case *dcrutil.AddressPubKeyHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		if addr.DSA(addr.Net()) != dcrec.STEcdsaSecp256k1 {
			str := "unable to generate payment script for " +
				"unsupported digital signature algorithm"
			return nil, scriptError(ErrUnsupportedAddress, str)
		}

	case *dcrutil.AddressScriptHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		scriptType = ScriptHashTy

	default:
		str := fmt.Sprintf("unable to generate payment script for "+
			"unsupported address type %T", addr)
		return nil, scriptError(ErrUnsupportedAddress, str)
	}

	hash := addr.ScriptAddress()

	if scriptType == PubKeyHashTy {
		return NewScriptBuilder().AddOp(OP_SSGEN).AddOp(OP_DUP).
			AddOp(OP_HASH160).AddData(hash).AddOp(OP_EQUALVERIFY).
			AddOp(OP_CHECKSIG).Script()
	}
	return NewScriptBuilder().AddOp(OP_SSGEN).AddOp(OP_HASH160).
		AddData(hash).AddOp(OP_EQUAL).Script()
}

// PayToSSGenPKHDirect creates a new script to pay a transaction output to a
// public key hash, but tags the output with OP_SSGEN. For use in constructing
// valid SSGen txs. Unlike PayToSSGen, this function directly uses the HASH160
// pubkeyhash (instead of an address).
func PayToSSGenPKHDirect(pkh []byte) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_SSGEN).AddOp(OP_DUP).
		AddOp(OP_HASH160).AddData(pkh).AddOp(OP_EQUALVERIFY).
		AddOp(OP_CHECKSIG).Script()
}

// PayToSSGenSHDirect creates a new script to pay a transaction output to a
// script hash, but tags the output with OP_SSGEN. For use in constructing
// valid SSGen txs. Unlike PayToSSGen, this function directly uses the HASH160
// script hash (instead of an address).
func PayToSSGenSHDirect(sh []byte) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_SSGEN).AddOp(OP_HASH160).
		AddData(sh).AddOp(OP_EQUAL).Script()
}

// PayToSSRtx creates a new script to pay a transaction output to a
// public key hash, but tags the output with OP_SSRTX. For use in constructing
// valid SSRtx.
func PayToSSRtx(addr dcrutil.Address) ([]byte, error) {
	// Only pay to pubkey hash and pay to script hash are
	// supported.
	scriptType := PubKeyHashTy
	switch addr := addr.(type) {
	case *dcrutil.AddressPubKeyHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		if addr.DSA(addr.Net()) != dcrec.STEcdsaSecp256k1 {
			str := "unable to generate payment script for " +
				"unsupported digital signature algorithm"
			return nil, scriptError(ErrUnsupportedAddress, str)
		}

	case *dcrutil.AddressScriptHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		scriptType = ScriptHashTy

	default:
		str := fmt.Sprintf("unable to generate payment script for "+
			"unsupported address type %T", addr)
		return nil, scriptError(ErrUnsupportedAddress, str)
	}

	hash := addr.ScriptAddress()

	if scriptType == PubKeyHashTy {
		return NewScriptBuilder().AddOp(OP_SSRTX).AddOp(OP_DUP).
			AddOp(OP_HASH160).AddData(hash).AddOp(OP_EQUALVERIFY).
			AddOp(OP_CHECKSIG).Script()
	}
	return NewScriptBuilder().AddOp(OP_SSRTX).AddOp(OP_HASH160).
		AddData(hash).AddOp(OP_EQUAL).Script()
}

// PayToSSRtxPKHDirect creates a new script to pay a transaction output to a
// public key hash, but tags the output with OP_SSRTX. For use in constructing
// valid SSRtx. Unlike PayToSSRtx, this function directly uses the HASH160
// pubkeyhash (instead of an address).
func PayToSSRtxPKHDirect(pkh []byte) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_SSRTX).AddOp(OP_DUP).
		AddOp(OP_HASH160).AddData(pkh).AddOp(OP_EQUALVERIFY).
		AddOp(OP_CHECKSIG).Script()
}

// PayToSSRtxSHDirect creates a new script to pay a transaction output to a
// script hash, but tags the output with OP_SSRTX. For use in constructing
// valid SSRtx. Unlike PayToSSRtx, this function directly uses the HASH160
// script hash (instead of an address).
func PayToSSRtxSHDirect(sh []byte) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_SSRTX).AddOp(OP_HASH160).
		AddData(sh).AddOp(OP_EQUAL).Script()
}

// GenerateSStxAddrPush generates an OP_RETURN push for SSGen payment addresses in
// an SStx.
func GenerateSStxAddrPush(addr dcrutil.Address, amount dcrutil.Amount, limits uint16) ([]byte, error) {
	// Only pay to pubkey hash and pay to script hash are
	// supported.
	scriptType := PubKeyHashTy
	switch addr := addr.(type) {
	case *dcrutil.AddressPubKeyHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		if addr.DSA(addr.Net()) != dcrec.STEcdsaSecp256k1 {
			str := "unable to generate payment script for " +
				"unsupported digital signature algorithm"
			return nil, scriptError(ErrUnsupportedAddress, str)
		}

	case *dcrutil.AddressScriptHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		scriptType = ScriptHashTy

	default:
		str := fmt.Sprintf("unable to generate payment script for "+
			"unsupported address type %T", addr)
		return nil, scriptError(ErrUnsupportedAddress, str)
	}

	// Concatenate the prefix, pubkeyhash, and amount.
	adBytes := make([]byte, 20+8+2)
	copy(adBytes[0:20], addr.ScriptAddress())
	binary.LittleEndian.PutUint64(adBytes[20:28], uint64(amount))
	binary.LittleEndian.PutUint16(adBytes[28:30], limits)

	// Set the bit flag indicating pay to script hash.
	if scriptType == ScriptHashTy {
		adBytes[27] |= 1 << 7
	}

	return NewScriptBuilder().AddOp(OP_RETURN).AddData(adBytes).Script()
}

// GenerateSSGenBlockRef generates an OP_RETURN push for the block header hash and
// height which the block votes on.
func GenerateSSGenBlockRef(blockHash chainhash.Hash, height uint32) ([]byte, error) {
	// Serialize the block hash and height
	brBytes := make([]byte, 32+4)
	copy(brBytes[0:32], blockHash[:])
	binary.LittleEndian.PutUint32(brBytes[32:36], height)

	return NewScriptBuilder().AddOp(OP_RETURN).AddData(brBytes).Script()
}

// GenerateSSGenVotes generates an OP_RETURN push for the vote bits in an SSGen tx.
func GenerateSSGenVotes(votebits uint16) ([]byte, error) {
	// Serialize the votebits
	vbBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(vbBytes, votebits)

	return NewScriptBuilder().AddOp(OP_RETURN).AddData(vbBytes).Script()
}

// GenerateProvablyPruneableOut creates a provably-prunable script containing
// OP_RETURN followed by the passed data.  An Error with the error code
// ErrTooMuchNullData will be returned if the length of the passed data exceeds
// MaxDataCarrierSize.
func GenerateProvablyPruneableOut(data []byte) ([]byte, error) {
	if len(data) > MaxDataCarrierSize {
		str := fmt.Sprintf("data size %d is larger than max "+
			"allowed size %d", len(data), MaxDataCarrierSize)
		return nil, scriptError(ErrTooMuchNullData, str)
	}

	return NewScriptBuilder().AddOp(OP_RETURN).AddData(data).Script()
}

// PayToAddrScript creates a new script to pay a transaction output to a the
// specified address.
func PayToAddrScript(addr dcrutil.Address) ([]byte, error) {
	switch addr := addr.(type) {
	case *dcrutil.AddressPubKeyHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		switch addr.DSA(addr.Net()) {
		case dcrec.STEcdsaSecp256k1:
			return payToPubKeyHashScript(addr.ScriptAddress())
		case dcrec.STEd25519:
			return payToPubKeyHashEdwardsScript(addr.ScriptAddress())
		case dcrec.STSchnorrSecp256k1:
			return payToPubKeyHashSchnorrScript(addr.ScriptAddress())
		}

	case *dcrutil.AddressScriptHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		return payToScriptHashScript(addr.ScriptAddress())

	case *dcrutil.AddressSecpPubKey:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		return payToPubKeyScript(addr.ScriptAddress())

	case *dcrutil.AddressEdwardsPubKey:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		return payToEdwardsPubKeyScript(addr.ScriptAddress())

	case *dcrutil.AddressSecSchnorrPubKey:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		return payToSchnorrPubKeyScript(addr.ScriptAddress())
	}

	str := fmt.Sprintf("unable to generate payment script for unsupported "+
		"address type %T", addr)
	return nil, scriptError(ErrUnsupportedAddress, str)
}

// MultiSigScript returns a valid script for a multisignature redemption where
// nrequired of the keys in pubkeys are required to have signed the transaction
// for success.  An Error with the error code ErrTooManyRequiredSigs will be
// returned if nrequired is larger than the number of keys provided.
func MultiSigScript(pubkeys []*dcrutil.AddressSecpPubKey, nrequired int) ([]byte, error) {
	if len(pubkeys) < nrequired {
		str := fmt.Sprintf("unable to generate multisig script with "+
			"%d required signatures when there are only %d public "+
			"keys available", nrequired, len(pubkeys))
		return nil, scriptError(ErrTooManyRequiredSigs, str)
	}

	builder := NewScriptBuilder().AddInt64(int64(nrequired))
	for _, key := range pubkeys {
		builder.AddData(key.ScriptAddress())
	}
	builder.AddInt64(int64(len(pubkeys)))
	builder.AddOp(OP_CHECKMULTISIG)

	return builder.Script()
}

// PushedData returns an array of byte slices containing any pushed data found
// in the passed script.  This includes OP_0, but not OP_1 - OP_16.
func PushedData(script []byte) ([][]byte, error) {
	pops, err := parseScript(script)
	if err != nil {
		return nil, err
	}

	var data [][]byte
	for _, pop := range pops {
		if pop.data != nil {
			data = append(data, pop.data)
		} else if pop.opcode.value == OP_0 {
			data = append(data, nil)
		}
	}
	return data, nil
}

// GetMultisigMandN returns the number of public keys and the number of
// signatures required to redeem the multisignature script.
//
// DEPRECATED.  Use CalcMultiSigStats instead.  This will be removed in the next
// major version bump.
func GetMultisigMandN(script []byte) (uint8, uint8, error) {
	numPubKeys, requiredSigs, err := CalcMultiSigStats(script)
	if err != nil {
		return 0, 0, err
	}

	return uint8(requiredSigs), uint8(numPubKeys), nil
}

// ExtractPkScriptAddrs returns the type of script, addresses and required
// signatures associated with the passed PkScript.  Note that it only works for
// 'standard' transaction script types.  Any data such as public keys which are
// invalid are omitted from the results.
func ExtractPkScriptAddrs(version uint16, pkScript []byte,
	chainParams *chaincfg.Params) (ScriptClass, []dcrutil.Address, int, error) {
	if version != DefaultScriptVersion {
		return NonStandardTy, nil, 0, fmt.Errorf("invalid script version")
	}

	var addrs []dcrutil.Address
	var requiredSigs int

	// No valid addresses or required signatures if the script doesn't
	// parse.
	pops, err := parseScript(pkScript)
	if err != nil {
		return NonStandardTy, nil, 0, err
	}

	scriptClass := typeOfScript(version, pkScript)

	switch scriptClass {
	case PubKeyHashTy:
		// A pay-to-pubkey-hash script is of the form:
		//  OP_DUP OP_HASH160 <hash> OP_EQUALVERIFY OP_CHECKSIG
		// Therefore the pubkey hash is the 3rd item on the stack.
		// Skip the pubkey hash if it's invalid for some reason.
		requiredSigs = 1
		addr, err := dcrutil.NewAddressPubKeyHash(pops[2].data,
			chainParams, dcrec.STEcdsaSecp256k1)
		if err == nil {
			addrs = append(addrs, addr)
		}

	case PubkeyHashAltTy:
		// A pay-to-pubkey-hash script is of the form:
		// OP_DUP OP_HASH160 <hash> OP_EQUALVERIFY <type> OP_CHECKSIGALT
		// Therefore the pubkey hash is the 3rd item on the stack.
		// Skip the pubkey hash if it's invalid for some reason.
		requiredSigs = 1
		suite, _ := ExtractPkScriptAltSigType(pkScript)
		addr, err := dcrutil.NewAddressPubKeyHash(pops[2].data,
			chainParams, suite)
		if err == nil {
			addrs = append(addrs, addr)
		}

	case PubKeyTy:
		// A pay-to-pubkey script is of the form:
		//  <pubkey> OP_CHECKSIG
		// Therefore the pubkey is the first item on the stack.
		// Skip the pubkey if it's invalid for some reason.
		requiredSigs = 1
		pk, err := secp256k1.ParsePubKey(pops[0].data)
		if err == nil {
			addr, err := dcrutil.NewAddressSecpPubKeyCompressed(pk, chainParams)
			if err == nil {
				addrs = append(addrs, addr)
			}
		}

	case PubkeyAltTy:
		// A pay-to-pubkey alt script is of the form:
		//  <pubkey> <type> OP_CHECKSIGALT
		// Therefore the pubkey is the first item on the stack.
		// Skip the pubkey if it's invalid for some reason.
		requiredSigs = 1
		suite, _ := ExtractPkScriptAltSigType(pkScript)
		var addr dcrutil.Address
		err := fmt.Errorf("invalid signature suite for alt sig")
		switch suite {
		case dcrec.STEd25519:
			addr, err = dcrutil.NewAddressEdwardsPubKey(pops[0].data,
				chainParams)
		case dcrec.STSchnorrSecp256k1:
			addr, err = dcrutil.NewAddressSecSchnorrPubKey(pops[0].data,
				chainParams)
		}
		if err == nil {
			addrs = append(addrs, addr)
		}

	case StakeSubmissionTy:
		// A pay-to-stake-submission-hash script is of the form:
		//  OP_SSTX ... P2PKH or P2SH
		var localAddrs []dcrutil.Address
		_, localAddrs, requiredSigs, err =
			ExtractPkScriptAddrs(version, getStakeOutSubscript(pkScript),
				chainParams)
		if err == nil {
			addrs = append(addrs, localAddrs...)
		}

	case StakeGenTy:
		// A pay-to-stake-generation-hash script is of the form:
		//  OP_SSGEN  ... P2PKH or P2SH
		var localAddrs []dcrutil.Address
		_, localAddrs, requiredSigs, err = ExtractPkScriptAddrs(version,
			getStakeOutSubscript(pkScript), chainParams)
		if err == nil {
			addrs = append(addrs, localAddrs...)
		}

	case StakeRevocationTy:
		// A pay-to-stake-revocation-hash script is of the form:
		//  OP_SSRTX  ... P2PKH or P2SH
		var localAddrs []dcrutil.Address
		_, localAddrs, requiredSigs, err =
			ExtractPkScriptAddrs(version, getStakeOutSubscript(pkScript),
				chainParams)
		if err == nil {
			addrs = append(addrs, localAddrs...)
		}

	case StakeSubChangeTy:
		// A pay-to-stake-submission-change-hash script is of the form:
		// OP_SSTXCHANGE ... P2PKH or P2SH
		var localAddrs []dcrutil.Address
		_, localAddrs, requiredSigs, err =
			ExtractPkScriptAddrs(version, getStakeOutSubscript(pkScript),
				chainParams)
		if err == nil {
			addrs = append(addrs, localAddrs...)
		}

	case ScriptHashTy:
		// A pay-to-script-hash script is of the form:
		//  OP_HASH160 <scripthash> OP_EQUAL
		// Therefore the script hash is the 2nd item on the stack.
		// Skip the script hash if it's invalid for some reason.
		requiredSigs = 1
		addr, err := dcrutil.NewAddressScriptHashFromHash(pops[1].data,
			chainParams)
		if err == nil {
			addrs = append(addrs, addr)
		}

	case MultiSigTy:
		// A multi-signature script is of the form:
		//  <numsigs> <pubkey> <pubkey> <pubkey>... <numpubkeys> OP_CHECKMULTISIG
		// Therefore the number of required signatures is the 1st item
		// on the stack and the number of public keys is the 2nd to last
		// item on the stack.
		requiredSigs = asSmallInt(pops[0].opcode.value)
		numPubKeys := asSmallInt(pops[len(pops)-2].opcode.value)

		// Extract the public keys while skipping any that are invalid.
		addrs = make([]dcrutil.Address, 0, numPubKeys)
		for i := 0; i < numPubKeys; i++ {
			pubkey, err := secp256k1.ParsePubKey(pops[i+1].data)
			if err == nil {
				addr, err := dcrutil.NewAddressSecpPubKeyCompressed(pubkey,
					chainParams)
				if err == nil {
					addrs = append(addrs, addr)
				}
			}
		}

	case NullDataTy:
		// Null data transactions have no addresses or required
		// signatures.

	case NonStandardTy:
		// Don't attempt to extract addresses or required signatures for
		// nonstandard transactions.
	}

	return scriptClass, addrs, requiredSigs, nil
}

// extractOneBytePush returns the value of a one byte push.
func extractOneBytePush(po parsedOpcode) int {
	if !isOneByteMaxDataPush(po) {
		return -1
	}

	if po.opcode.value == OP_1 ||
		po.opcode.value == OP_2 ||
		po.opcode.value == OP_3 ||
		po.opcode.value == OP_4 ||
		po.opcode.value == OP_5 ||
		po.opcode.value == OP_6 ||
		po.opcode.value == OP_7 ||
		po.opcode.value == OP_8 ||
		po.opcode.value == OP_9 ||
		po.opcode.value == OP_10 ||
		po.opcode.value == OP_11 ||
		po.opcode.value == OP_12 ||
		po.opcode.value == OP_13 ||
		po.opcode.value == OP_14 ||
		po.opcode.value == OP_15 ||
		po.opcode.value == OP_16 {
		return int(po.opcode.value - 80)
	}

	return int(po.data[0])
}

// ExtractPkScriptAltSigType returns the signature scheme to use for an
// alternative check signature script.
func ExtractPkScriptAltSigType(pkScript []byte) (dcrec.SignatureType, error) {
	pops, err := parseScript(pkScript)
	if err != nil {
		return 0, err
	}

	isPKA := isPubkeyAlt(pops)
	isPKHA := isPubkeyHashAlt(pops)
	if !(isPKA || isPKHA) {
		return -1, fmt.Errorf("wrong script type")
	}

	sigTypeLoc := 1
	if isPKHA {
		sigTypeLoc = 4
	}

	valInt := extractOneBytePush(pops[sigTypeLoc])
	if valInt < 0 {
		return 0, fmt.Errorf("bad type push")
	}
	val := dcrec.SignatureType(valInt)
	switch val {
	case dcrec.STEd25519:
		return val, nil
	case dcrec.STSchnorrSecp256k1:
		return val, nil
	default:
		break
	}

	return -1, fmt.Errorf("bad signature scheme type")
}

// AtomicSwapDataPushes houses the data pushes found in atomic swap contracts.
type AtomicSwapDataPushes struct {
	RecipientHash160 [20]byte
	RefundHash160    [20]byte
	SecretHash       [32]byte
	SecretSize       int64
	LockTime         int64
}

// ExtractAtomicSwapDataPushes returns the data pushes from an atomic swap
// contract.  If the script is not an atomic swap contract,
// ExtractAtomicSwapDataPushes returns (nil, nil).  Non-nil errors are returned
// for unparsable scripts.
//
// NOTE: Atomic swaps are not considered standard script types by the dcrd
// mempool policy and should be used with P2SH.  The atomic swap format is also
// expected to change to use a more secure hash function in the future.
//
// This function is only defined in the txscript package due to API limitations
// which prevent callers using txscript to parse nonstandard scripts.
func ExtractAtomicSwapDataPushes(version uint16, pkScript []byte) (*AtomicSwapDataPushes, error) {
	pops, err := parseScript(pkScript)
	if err != nil {
		return nil, err
	}

	if len(pops) != 20 {
		return nil, nil
	}
	isAtomicSwap := pops[0].opcode.value == OP_IF &&
		pops[1].opcode.value == OP_SIZE &&
		canonicalPush(pops[2]) &&
		pops[3].opcode.value == OP_EQUALVERIFY &&
		pops[4].opcode.value == OP_SHA256 &&
		pops[5].opcode.value == OP_DATA_32 &&
		pops[6].opcode.value == OP_EQUALVERIFY &&
		pops[7].opcode.value == OP_DUP &&
		pops[8].opcode.value == OP_HASH160 &&
		pops[9].opcode.value == OP_DATA_20 &&
		pops[10].opcode.value == OP_ELSE &&
		canonicalPush(pops[11]) &&
		pops[12].opcode.value == OP_CHECKLOCKTIMEVERIFY &&
		pops[13].opcode.value == OP_DROP &&
		pops[14].opcode.value == OP_DUP &&
		pops[15].opcode.value == OP_HASH160 &&
		pops[16].opcode.value == OP_DATA_20 &&
		pops[17].opcode.value == OP_ENDIF &&
		pops[18].opcode.value == OP_EQUALVERIFY &&
		pops[19].opcode.value == OP_CHECKSIG
	if !isAtomicSwap {
		return nil, nil
	}

	pushes := new(AtomicSwapDataPushes)
	copy(pushes.SecretHash[:], pops[5].data)
	copy(pushes.RecipientHash160[:], pops[9].data)
	copy(pushes.RefundHash160[:], pops[16].data)
	if pops[2].data != nil {
		locktime, err := makeScriptNum(pops[2].data, 5)
		if err != nil {
			return nil, nil
		}
		pushes.SecretSize = int64(locktime)
	} else if op := pops[2].opcode; isSmallInt(op.value) {
		pushes.SecretSize = int64(asSmallInt(op.value))
	} else {
		return nil, nil
	}
	if pops[11].data != nil {
		locktime, err := makeScriptNum(pops[11].data, 5)
		if err != nil {
			return nil, nil
		}
		pushes.LockTime = int64(locktime)
	} else if op := pops[11].opcode; isSmallInt(op.value) {
		pushes.LockTime = int64(asSmallInt(op.value))
	} else {
		return nil, nil
	}
	return pushes, nil
}
