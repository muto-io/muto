package tenant

type IsolationTier string

const (
	TierShared    IsolationTier = "shared"
	TierDedicated IsolationTier = "dedicated"
)

type Tenant struct {
	ID            string
	Namespace     string
	IsolationTier IsolationTier
	BusType       string
	BusDedicated  bool
}
