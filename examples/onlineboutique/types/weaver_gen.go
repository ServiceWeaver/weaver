// go:build !ignoreWeaverGen

package types

// Code generated by "weaver generate". DO NOT EDIT.
import (
	"fmt"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
)


// Local stub implementations.

// Client stub implementations.

// Server stub implementations.

// AutoMarshal implementations.

var _ codegen.AutoMarshal = &Order{}

func (x *Order) WeaverMarshal(enc *codegen.Encoder) {
	if x == nil {
		panic(fmt.Errorf("Order.WeaverMarshal: nil receiver"))
	}
	enc.String(x.OrderID)
	enc.String(x.ShippingTrackingID)
	(x.ShippingCost).WeaverMarshal(enc)
	(x.ShippingAddress).WeaverMarshal(enc)
	serviceweaver_enc_slice_OrderItem_2b9377cb(enc, x.Items)
}

func (x *Order) WeaverUnmarshal(dec *codegen.Decoder) {
	if x == nil {
		panic(fmt.Errorf("Order.WeaverUnmarshal: nil receiver"))
	}
	x.OrderID = dec.String()
	x.ShippingTrackingID = dec.String()
	(&x.ShippingCost).WeaverUnmarshal(dec)
	(&x.ShippingAddress).WeaverUnmarshal(dec)
	x.Items = serviceweaver_dec_slice_OrderItem_2b9377cb(dec)
}

func serviceweaver_enc_slice_OrderItem_2b9377cb(enc *codegen.Encoder, arg []OrderItem) {
	if arg == nil {
		enc.Len(-1)
		return
	}
	enc.Len(len(arg))
	for i := 0; i < len(arg); i++ {
		(arg[i]).WeaverMarshal(enc)
	}
}

func serviceweaver_dec_slice_OrderItem_2b9377cb(dec *codegen.Decoder) []OrderItem {
	n := dec.Len()
	if n == -1 {
		return nil
	}
	res := make([]OrderItem, n)
	for i := 0; i < n; i++ {
		(&res[i]).WeaverUnmarshal(dec)
	}
	return res
}

var _ codegen.AutoMarshal = &OrderItem{}

func (x *OrderItem) WeaverMarshal(enc *codegen.Encoder) {
	if x == nil {
		panic(fmt.Errorf("OrderItem.WeaverMarshal: nil receiver"))
	}
	(x.Item).WeaverMarshal(enc)
	(x.Cost).WeaverMarshal(enc)
}

func (x *OrderItem) WeaverUnmarshal(dec *codegen.Decoder) {
	if x == nil {
		panic(fmt.Errorf("OrderItem.WeaverUnmarshal: nil receiver"))
	}
	(&x.Item).WeaverUnmarshal(dec)
	(&x.Cost).WeaverUnmarshal(dec)
}
