package sqlitedb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ark-network/ark/server/internal/core/domain"
	"github.com/ark-network/ark/server/internal/infrastructure/db/sqlite/sqlc/queries"
)

type vxtoRepository struct {
	db      *sql.DB
	querier *queries.Queries
}

func NewVtxoRepository(config ...interface{}) (domain.VtxoRepository, error) {
	if len(config) != 1 {
		return nil, fmt.Errorf("invalid config")
	}
	db, ok := config[0].(*sql.DB)
	if !ok {
		return nil, fmt.Errorf("cannot open vtxo repository: invalid config")
	}

	return &vxtoRepository{
		db:      db,
		querier: queries.New(db),
	}, nil
}

func (v *vxtoRepository) Close() {
	_ = v.db.Close()
}

func (v *vxtoRepository) AddVtxos(ctx context.Context, vtxos []domain.Vtxo) error {
	txBody := func(querierWithTx *queries.Queries) error {
		for i := range vtxos {
			vtxo := vtxos[i]

			if err := querierWithTx.UpsertVtxo(
				ctx, queries.UpsertVtxoParams{
					Txid:      vtxo.Txid,
					Vout:      int64(vtxo.VOut),
					Pubkey:    vtxo.PubKey,
					Amount:    int64(vtxo.Amount),
					RoundTx:   vtxo.RoundTxid,
					SpentBy:   vtxo.SpentBy,
					Spent:     vtxo.Spent,
					Redeemed:  vtxo.Redeemed,
					Swept:     vtxo.Swept,
					ExpireAt:  vtxo.ExpireAt,
					CreatedAt: vtxo.CreatedAt,
					RedeemTx:  sql.NullString{String: vtxo.RedeemTx, Valid: true},
				},
			); err != nil {
				return err
			}
		}

		return nil
	}

	return execTx(ctx, v.db, txBody)
}

func (v *vxtoRepository) GetAllSweepableVtxos(ctx context.Context) ([]domain.Vtxo, error) {
	res, err := v.querier.SelectSweepableVtxos(ctx)
	if err != nil {
		return nil, err
	}

	rows := make([]queries.Vtxo, 0, len(res))
	for _, row := range res {
		rows = append(rows, row.Vtxo)
	}
	return readRows(rows)
}

func (v *vxtoRepository) GetAllVtxos(ctx context.Context, pubkey string) ([]domain.Vtxo, []domain.Vtxo, error) {
	withPubkey := len(pubkey) > 0

	var rows []queries.Vtxo
	if withPubkey {
		res, err := v.querier.SelectNotRedeemedVtxosWithPubkey(ctx, pubkey)
		if err != nil {
			return nil, nil, err
		}
		rows = make([]queries.Vtxo, 0, len(res))
		for _, row := range res {
			rows = append(rows, row.Vtxo)
		}
	} else {
		res, err := v.querier.SelectNotRedeemedVtxos(ctx)
		if err != nil {
			return nil, nil, err
		}
		rows = make([]queries.Vtxo, 0, len(res))
		for _, row := range res {
			rows = append(rows, row.Vtxo)
		}
	}

	vtxos, err := readRows(rows)
	if err != nil {
		return nil, nil, err
	}

	unspentVtxos := make([]domain.Vtxo, 0)
	spentVtxos := make([]domain.Vtxo, 0)

	for _, vtxo := range vtxos {
		if vtxo.Spent || vtxo.Swept {
			spentVtxos = append(spentVtxos, vtxo)
		} else {
			unspentVtxos = append(unspentVtxos, vtxo)
		}
	}

	return unspentVtxos, spentVtxos, nil
}

func (v *vxtoRepository) GetVtxos(ctx context.Context, outpoints []domain.VtxoKey) ([]domain.Vtxo, error) {
	vtxos := make([]domain.Vtxo, 0, len(outpoints))
	for _, o := range outpoints {
		res, err := v.querier.SelectVtxoByOutpoint(
			ctx,
			queries.SelectVtxoByOutpointParams{
				Txid: o.Txid,
				Vout: int64(o.VOut),
			},
		)
		if err != nil {
			return nil, err
		}

		result, err := readRows([]queries.Vtxo{res.Vtxo})
		if err != nil {
			return nil, err
		}

		if len(result) == 0 {
			return nil, fmt.Errorf("vtxo not found")
		}

		vtxos = append(vtxos, result[0])
	}

	return vtxos, nil
}

func (v *vxtoRepository) GetVtxosForRound(ctx context.Context, txid string) ([]domain.Vtxo, error) {
	res, err := v.querier.SelectVtxosByRoundTxid(ctx, txid)
	if err != nil {
		return nil, err
	}
	rows := make([]queries.Vtxo, 0, len(res))
	for _, row := range res {
		rows = append(rows, row.Vtxo)
	}

	return readRows(rows)
}

func (v *vxtoRepository) RedeemVtxos(ctx context.Context, vtxos []domain.VtxoKey) error {
	txBody := func(querierWithTx *queries.Queries) error {
		for _, vtxo := range vtxos {
			if err := querierWithTx.MarkVtxoAsRedeemed(
				ctx,
				queries.MarkVtxoAsRedeemedParams{
					Txid: vtxo.Txid,
					Vout: int64(vtxo.VOut),
				},
			); err != nil {
				return err
			}
		}

		return nil
	}

	return execTx(ctx, v.db, txBody)
}

func (v *vxtoRepository) SpendVtxos(ctx context.Context, vtxos []domain.VtxoKey, txid string) error {
	txBody := func(querierWithTx *queries.Queries) error {
		for _, vtxo := range vtxos {
			if err := querierWithTx.MarkVtxoAsSpent(
				ctx,
				queries.MarkVtxoAsSpentParams{
					SpentBy: txid,
					Txid:    vtxo.Txid,
					Vout:    int64(vtxo.VOut),
				},
			); err != nil {
				return err
			}
		}

		return nil
	}

	return execTx(ctx, v.db, txBody)
}

func (v *vxtoRepository) SweepVtxos(ctx context.Context, vtxos []domain.VtxoKey) error {
	txBody := func(querierWithTx *queries.Queries) error {
		for _, vtxo := range vtxos {
			if err := querierWithTx.MarkVtxoAsSwept(
				ctx,
				queries.MarkVtxoAsSweptParams{
					Txid: vtxo.Txid,
					Vout: int64(vtxo.VOut),
				},
			); err != nil {
				return err
			}
		}

		return nil
	}

	return execTx(ctx, v.db, txBody)
}

func (v *vxtoRepository) UpdateExpireAt(ctx context.Context, vtxos []domain.VtxoKey, expireAt int64) error {
	txBody := func(querierWithTx *queries.Queries) error {
		for _, vtxo := range vtxos {
			if err := querierWithTx.UpdateVtxoExpireAt(
				ctx,
				queries.UpdateVtxoExpireAtParams{
					ExpireAt: expireAt,
					Txid:     vtxo.Txid,
					Vout:     int64(vtxo.VOut),
				},
			); err != nil {
				return err
			}
		}

		return nil
	}

	return execTx(ctx, v.db, txBody)
}

func rowToVtxo(row queries.Vtxo) domain.Vtxo {
	return domain.Vtxo{
		VtxoKey: domain.VtxoKey{
			Txid: row.Txid,
			VOut: uint32(row.Vout),
		},
		Amount:    uint64(row.Amount),
		PubKey:    row.Pubkey,
		RoundTxid: row.RoundTx,
		SpentBy:   row.SpentBy,
		Spent:     row.Spent,
		Redeemed:  row.Redeemed,
		Swept:     row.Swept,
		ExpireAt:  row.ExpireAt,
		RedeemTx:  row.RedeemTx.String,
		CreatedAt: row.CreatedAt,
	}
}

func readRows(rows []queries.Vtxo) ([]domain.Vtxo, error) {
	vtxos := make([]domain.Vtxo, 0, len(rows))
	for _, vtxo := range rows {
		vtxos = append(vtxos, rowToVtxo(vtxo))
	}

	return vtxos, nil
}


func (r *vxtoRepository) GetVtxosForUpdate(ctx context.Context, keys []domain.VtxoKey) ([]domain.Vtxo, error) {
    // Build a query with FOR UPDATE to lock the rows
    query := `SELECT txid, vout, amount, pubkey, spent, redeemed, swept, round_txid, 
              spent_by, expire_at, redeem_tx, created_at
              FROM vtxos 
              WHERE (txid, vout) IN (` + r.createInClause(len(keys)) + `)
              FOR UPDATE`
              
    // Map the VtxoKeys to flat arguments for the query
    args := make([]interface{}, 0, len(keys)*2)
    for _, key := range keys {
        args = append(args, key.Txid, key.VOut)
    }
    
    // Execute the query
    rows, err := r.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    // Parse results
    var vtxos []domain.Vtxo
    for rows.Next() {
        var v domain.Vtxo
        if err := rows.Scan(&v.Txid, &v.VOut, &v.Amount, &v.PubKey, &v.Spent, 
                           &v.Redeemed, &v.Swept, &v.RoundTxid, &v.SpentBy, 
                           &v.ExpireAt, &v.RedeemTx, &v.CreatedAt); err != nil {
            return nil, err
        }
        vtxos = append(vtxos, v)
    }
    
    return vtxos, nil
}

func (r *vxtoRepository) createInClause(n int) string {
    // Helper to create SQL IN clause with parameter placeholders
    placeholders := make([]string, n)
    for i := 0; i < n; i++ {
        placeholders[i] = "(?, ?)"
    }
    return strings.Join(placeholders, ", ")
}
