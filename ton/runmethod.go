package ton

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tl"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/tvm/cell"
)

func (c *APIClient) RunGetMethod(ctx context.Context, blockInfo *tlb.BlockInfo, addr *address.Address, method string, params ...any) ([]interface{}, error) {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, 0b00000100)

	data = append(data, blockInfo.Serialize()...)

	chain := make([]byte, 4)
	binary.LittleEndian.PutUint32(chain, uint32(addr.Workchain()))

	data = append(data, chain...)
	data = append(data, addr.Data()...)

	mName := make([]byte, 8)
	binary.LittleEndian.PutUint64(mName, tlb.MethodNameHash(method))
	data = append(data, mName...)

	var stack tlb.Stack
	for i := len(params) - 1; i >= 0; i-- {
		// push args in reverse order
		stack.Push(params[i])
	}

	req, err := stack.ToCell()
	if err != nil {
		return nil, fmt.Errorf("build stack err: %w", err)
	}

	// param
	data = append(data, tl.ToBytes(req.ToBOCWithFlags(false))...)

	resp, err := c.client.Do(ctx, _RunContractGetMethod, data)
	if err != nil {
		return nil, err
	}

	switch resp.TypeID {
	case _RunQueryResult:
		// TODO: mode
		_ = binary.LittleEndian.Uint32(resp.Data)

		resp.Data = resp.Data[4:]

		b := new(tlb.BlockInfo)
		resp.Data, err = b.Load(resp.Data)
		if err != nil {
			return nil, err
		}

		shard := new(tlb.BlockInfo)
		resp.Data, err = shard.Load(resp.Data)
		if err != nil {
			return nil, err
		}

		// TODO: check proofs mode

		exitCode := binary.LittleEndian.Uint32(resp.Data)
		if exitCode > 1 {
			return nil, ContractExecError{
				exitCode,
			}
		}
		resp.Data = resp.Data[4:]

		var state []byte
		state, resp.Data = loadBytes(resp.Data)

		cl, err := cell.FromBOC(state)
		if err != nil {
			return nil, err
		}

		var resStack tlb.Stack
		err = resStack.LoadFromCell(cl.BeginParse())
		if err != nil {
			return nil, err
		}

		var result []any

		for resStack.Depth() > 0 {
			v, _ := resStack.Pop()
			result = append(result, v)
		}

		// it comes in reverse order from lite server, reverse it back
		reversed := make([]any, len(result))
		for i := 0; i < len(result); i++ {
			reversed[(len(result)-1)-i] = result[i]
		}

		return result, nil
	case _LSError:
		var lsErr LSError
		resp.Data, err = lsErr.Load(resp.Data)
		if err != nil {
			return nil, err
		}
		return nil, lsErr
	}

	return nil, errors.New("unknown response type")
}
