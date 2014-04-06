package main

import (
	"math/big"
	"code.google.com/p/godec/dec"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

var ErrNotANumber = errors.New("not a number")
var timeFormat = "2006-01-02 15:04:05 -0700"
var dateFormat = "2006-01-02"
var validDescription = regexp.MustCompile(`(Bought|Sold) ([0-9.]+) BTC (for )+\$([0-9,.]+)\.`)

func NewDec(s string) (*dec.Dec, error) {
	d, success := new(dec.Dec).SetString(s)
	if !success {
		return nil, ErrNotANumber
	}
	return d, nil
}

type Trade struct {
	Timestamp		time.Time
	BtcAmount		*dec.Dec
	TotalPrice		*dec.Dec
	PricePerBitcoin *dec.Dec
	TransferFee		*dec.Dec
}

func NewTrade(t time.Time, b *dec.Dec, u *dec.Dec, f *dec.Dec) *Trade {
	trade := new(Trade)
	trade.Timestamp = t
	trade.BtcAmount = b
	trade.TotalPrice = u
	trade.TransferFee = f
	originalPrice := new(dec.Dec).Sub(trade.TotalPrice, trade.TransferFee)
	trade.PricePerBitcoin = new(dec.Dec).Abs(new(dec.Dec).Quo(originalPrice, trade.BtcAmount, 
		dec.Scale(2), dec.RoundHalfUp))
	return trade
}

func (t Trade) IsBuy() bool {
	return t.BtcAmount.Sign() > 0
}

type CoinbaseCsvReader struct {
	reader 		*csv.Reader
	headersRead bool
}

func NewCoinbaseCsvReader(r *csv.Reader) *CoinbaseCsvReader {
	c := new(CoinbaseCsvReader)
	c.reader = r
	c.reader.FieldsPerRecord = -1
	return c
}

func (c *CoinbaseCsvReader) Read() (*Trade, error) {
	if !c.headersRead {
		for i := 0; i < 3; i++ {
			_, err := c.reader.Read()
			if err != nil {
				return nil, err
			}
		}
		c.headersRead = true
	}

	for {
		row, err := c.reader.Read()
		if err != nil {
			return nil, err
		}

		t, err := time.Parse(timeFormat, row[0])
		if err != nil {
			return nil, err
		}

		b, err := NewDec(row[2])
		if err != nil {
			return nil, err
		}

		var u, f *dec.Dec
		if row[5] == "" {
			/* For some reason, the format changes in the middle of the exported transactions
			   list. The columns for USD total and fee are blank. The total is in the 
			   description, and we can calculate the fee based on our insider knowledge of
			   Coinbase's fee structure. 
			*/
			matches := validDescription.FindStringSubmatch(row[4])
			if len(matches) > 0 {
				usd_no_fucking_commas := strings.Replace(matches[4], ",", "", -1)
				u, err = NewDec(usd_no_fucking_commas)
				if err != nil {
					return nil, err
				}
				bankFee := dec.NewDec(big.NewInt(15), dec.Scale(2))
				coinbaseFeePercent := dec.NewDec(big.NewInt(101), dec.Scale(2))
				subtotal := new(dec.Dec).Quo(new(dec.Dec).Sub(u, bankFee), coinbaseFeePercent, 
					dec.Scale(2), dec.RoundHalfUp)
				f = new(dec.Dec).Sub(u, subtotal)
			} else {
				continue
			}
		} else {
			u, err = NewDec(row[5])
			if err != nil {
				return nil, err
			}

			f, err = NewDec(row[7])
			if err != nil {
				return nil, err
			}
		}

		return NewTrade(t, b, u, f), nil
	}
	panic("This can't happen")
}

type LedgerDatWriter struct {
	writer 		io.WriteCloser
	trades		[]*Trade
}

func NewLedgerDatWriter(w io.WriteCloser) *LedgerDatWriter {
	l := new(LedgerDatWriter)
	l.writer = w
	l.trades = make([]*Trade, 0)
	return l
}

func (l *LedgerDatWriter) writeString(s string) error {
	_, err := l.writer.Write([]byte(s))
	return err
}

func (l *LedgerDatWriter) Write(t *Trade) error {
	var entry string
	if t.IsBuy() {
		l.trades = append(l.trades, t)
		entry = fmt.Sprintf("%s\tBitcoin bought\n", t.Timestamp.Format(dateFormat)) +
			fmt.Sprintf("\tAssets:Coinbase\t%s BTC {$ %s}\n", t.BtcAmount, t.PricePerBitcoin) +
			fmt.Sprintf("\tAssets:Cash\t-$ %s\n", t.TotalPrice) +
			fmt.Sprintf("\tExpenses:Fees\t$ %s\n\n", t.TransferFee)
	} else {
		entry = fmt.Sprintf("%s\tBitcoin sold\n", t.Timestamp.Format(dateFormat))
		lotSize := new(dec.Dec)
		coinsToSell := new(dec.Dec)
		capitalGains := new(dec.Dec).Set(t.TotalPrice)
		for coinsToSell.Neg(t.BtcAmount); coinsToSell.Sign() > 0; coinsToSell.Add(coinsToSell, lotSize) {
			b := l.trades[0]
			if coinsToSell.Cmp(b.BtcAmount) < 0 {
				lotSize.Neg(coinsToSell)
				b.BtcAmount.Add(b.BtcAmount, lotSize)
			} else {
				lotSize.Neg(b.BtcAmount)
				l.trades = l.trades[1:]
			}
			lotPrice := new(dec.Dec).Mul(new(dec.Dec).Neg(lotSize), b.PricePerBitcoin)
			lotPrice.Round(lotPrice, dec.Scale(2), dec.RoundHalfUp)
			capitalGains.Sub(capitalGains, lotPrice)
			entry += fmt.Sprintf("\tAssets:Coinbase\t%s BTC {$ %s} @ $ %s\n", lotSize, b.PricePerBitcoin, 
				t.PricePerBitcoin)
		}
		capitalGains.Add(capitalGains, t.TransferFee)
		entry += fmt.Sprintf("\tAssets:Cash\t$ %s\n", t.TotalPrice) +
			fmt.Sprintf("\tExpenses:Fees\t$ %s\n", t.TransferFee) +
			fmt.Sprintf("\tIncome:Capital Gains\t$ -%s\n\n", capitalGains)
	}
	return l.writeString(entry)
}

func main() {
	file, err := os.Open("coinbase.csv")
	if err != nil {
		panic(err)
	}
	csv_reader := csv.NewReader(file)
	coinbase_reader := NewCoinbaseCsvReader(csv_reader)

	file, err = os.Create("ledger.dat")
	if err != nil {
		panic(err)
	}
	ledger_writer := NewLedgerDatWriter(file)

	var t *Trade
	for {
		t, err = coinbase_reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		err = ledger_writer.Write(t)
		if err != nil {
			panic(err)
		}
	}
}