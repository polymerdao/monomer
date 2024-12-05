package types

type DepositMsg interface {
	isDeposit()
}

func (*MsgSetL1Attributes) isDeposit()  {}
func (*MsgApplyUserDeposit) isDeposit() {}
