package txbuilder

import (
	"context"
	"time"

	"chain/errors"
	chainlog "chain/log"
	"chain/metrics"
	"chain/net/rpc"
	"chain/protocol"
	"chain/protocol/bc"
	"chain/protocol/validation"
)

var (
	// ErrRejected means the network rejected a tx (as a double-spend)
	ErrRejected = errors.New("transaction rejected")

	ErrMissingRawTx  = errors.New("missing raw tx")
	ErrBadInputCount = errors.New("too many inputs in template")
)

var Generator *rpc.Client

// FinalizeTx validates a transaction signature template,
// assembles a fully signed tx, and stores the effects of
// its changes on the UTXO set.
func FinalizeTx(ctx context.Context, c *protocol.Chain, tx *bc.Tx) error {
	defer metrics.RecordElapsed(time.Now())

	err := publishTx(ctx, c, tx)
	if err != nil {
		rawtx, err2 := tx.MarshalText()
		if err2 != nil {
			// ignore marshalling errors (they should never happen anyway)
			return err
		}
		return errors.Wrapf(err, "tx=%s", rawtx)
	}

	return nil
}

func publishTx(ctx context.Context, c *protocol.Chain, msg *bc.Tx) error {
	// Make sure there is atleast one block in case client is
	// trying to finalize a tx before the initial block has landed
	c.WaitForBlock(1)
	err := c.AddTx(ctx, msg)
	if errors.Root(err) == validation.ErrBadTx {
		detail := errors.Detail(err)
		err = errors.Wrap(ErrRejected, err)
		return errors.WithDetail(err, detail)
	} else if err != nil {
		return errors.Wrap(err, "add tx to blockchain")
	}

	if Generator != nil {
		err = Generator.Call(ctx, "/rpc/submit", msg, nil)
		if err != nil {
			err = errors.Wrap(err, "generator transaction notice")
			chainlog.Error(ctx, err)

			// Return an error so that the client knows that it needs to
			// retry the request.
			return err
		}
	}
	return nil
}
