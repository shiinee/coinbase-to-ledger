package main

import (
	"code.google.com/p/godec/dec"
	"encoding/csv"
	"errors"
	_"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

var ErrNotANumber = errors.New("not a number")
var timeFormat = "2006-01-02 15:04:05 -0700"
var validDescription = regexp.MustCompile(`(Bought|Sold) ([0-9.]+) BTC (for )+\$([0-9,.]+)\.`)

type Btc struct {
	dec.Dec
}

func NewBtc(s string) (*Btc, error) {
	b := new(Btc)
	_, success := b.SetString(s)
	if !success {
		return nil, ErrNotANumber
	}
	return b, nil
}

type Usd struct {
	dec.Dec
}

func NewUsd(s string) (*Usd, error) {
	u := new(Usd)
	_, success := u.SetString(s)
	if !success {
		return nil, ErrNotANumber
	}
	return u, nil
}

type Trade struct {
	Timestamp	time.Time
	Amount		*Btc
	Price		*Usd
	// fee
}

func NewTrade(t time.Time, b *Btc, u *Usd) *Trade {
	trade := new(Trade)
	trade.Timestamp = t
	trade.Amount = b
	trade.Price = u
	return trade
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

		b, err := NewBtc(row[2])
		if err != nil {
			return nil, err
		}

		var u *Usd
		if row[5] == "" {
			matches := validDescription.FindStringSubmatch(row[4])
			if len(matches) > 0 {
				usd_no_fucking_commas := strings.Replace(matches[4], ",", "", -1)
				u, err = NewUsd(usd_no_fucking_commas)
				if err != nil {
					return nil, err
				}
			} else {
				continue
			}
		} else {
			u, err = NewUsd(row[5])
			if err != nil {
				return nil, err
			}
		}

		return NewTrade(t, b, u), nil
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

func (l *LedgerDatWriter) Write(t *Trade) error {
	l.writer.Write([]byte("HI"))
	return nil
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