package ports

type Clock interface{ Now() int64 }
