package sale

import "context"

// ItemQuery reuses the document filters and adds the line-level sort. Sorting
// is whitelisted in the Service so user text never reaches the SQL writer.
type ItemQuery struct {
	Query
	SortBy  string
	SortDir string
}

// ItemRecord is one saved sale line with the returns still in effect for it.
// Net values stay null while they are not applicable: a draft or cancelled
// sale keeps its original quantity and amount for querying, but it is not an
// effective sale, and a service line cannot be returned at all.
type ItemRecord struct {
	NetQuantity          *string `json:"netQuantity"`
	NetAmount            *string `json:"netAmount"`
	ProductModel         *string `json:"productModel"`
	ProductSpecification *string `json:"productSpecification"`
	MatchesHistory       bool    `json:"matchesHistory,omitempty"`
	ID                   int64   `json:"id"`
	SaleID               int64   `json:"saleId"`
	DocumentNo           string  `json:"documentNo"`
	PartnerID            int64   `json:"partnerId"`
	PartnerName          string  `json:"partnerName"`
	BusinessDate         string  `json:"businessDate"`
	Status               string  `json:"status"`
	ProductID            int64   `json:"productId"`
	ProductCode          string  `json:"productCode"`
	ProductName          string  `json:"productName"`
	ProductType          string  `json:"productType"`
	Unit                 string  `json:"unit"`
	Quantity             string  `json:"quantity"`
	UnitPrice            string  `json:"unitPrice"`
	Amount               string  `json:"amount"`
	ReturnedQuantity     string  `json:"returnedQuantity"`
	ReturnedAmount       string  `json:"returnedAmount"`
	ReturnsApplicable    bool    `json:"returnsApplicable"`
	NetApplicable        bool    `json:"netApplicable"`
}

// ItemAmounts sums the original line amounts of one sale status. Returned and
// net amounts only exist for posted sales, which are the only effective ones.
type ItemAmounts struct {
	ReturnedAmount *string `json:"returnedAmount"`
	NetAmount      *string `json:"netAmount"`
	Status         string  `json:"status"`
	ItemCount      int64   `json:"itemCount"`
	OriginalAmount string  `json:"originalAmount"`
}

// ItemQuantity sums one product and unit within one status. Groups are keyed
// by the internal product identity, so a renamed archive keeps one group, and
// the description is taken from the group's own saved snapshots.
type ItemQuantity struct {
	ReturnedQuantity  *string `json:"returnedQuantity"`
	NetQuantity       *string `json:"netQuantity"`
	ProductID         int64   `json:"productId"`
	ProductCode       string  `json:"productCode"`
	ProductName       string  `json:"productName"`
	ProductType       string  `json:"productType"`
	Unit              string  `json:"unit"`
	Status            string  `json:"status"`
	Quantity          string  `json:"quantity"`
	ReturnsApplicable bool    `json:"returnsApplicable"`
}

type ItemSummary struct {
	Amounts    []ItemAmounts  `json:"amounts"`
	Quantities []ItemQuantity `json:"quantities"`
}

// ItemPage is one page of matching sale lines plus the summary of every
// matching line, not just this page.
type ItemPage struct {
	Records  []ItemRecord `json:"records"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
	Summary  ItemSummary  `json:"summary"`
}

func (s *Service) ItemPage(ctx context.Context, q ItemQuery) (ItemPage, error) {
	q.Page, q.PageSize = pageBounds(q.Page, q.PageSize)
	if e := validateQueryFilters(q.Query); e != nil {
		return ItemPage{}, e
	}
	var e error
	if q.SortBy, q.SortDir, e = itemSort(q.SortBy, q.SortDir); e != nil {
		return ItemPage{}, e
	}
	return s.store.ItemPage(ctx, q)
}

func itemSort(field, direction string) (string, string, error) {
	switch field {
	case "":
		field = "businessDate"
	case "businessDate", "unitPrice", "amount", "netAmount":
	default:
		return "", "", ErrInvalid
	}
	switch direction {
	case "":
		direction = "desc"
	case "asc", "desc":
	default:
		return "", "", ErrInvalid
	}
	return field, direction, nil
}

func statusRank(status string) int {
	switch status {
	case "DRAFT":
		return 0
	case "POSTED":
		return 1
	case "CANCELLED":
		return 2
	}
	return 3
}
