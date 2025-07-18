package utils

import (
	"fmt"
	"testing"
	"time"

	"github.com/bytedance/sonic/ast"
)

func TestDo(t *testing.T) {
	var opts = RequestOptions{
		Method: "GET",
		URL:    "http://api.open-notify.org/iss-now.json",
		// Query: map[string]interface{}{
		// 	"format": "json",
		// },
		Timeout: 20 * time.Second,
	}
	httpResp := Do(opts)
	if httpResp.Error != nil {
		t.Errorf("Error making request: %v", httpResp.Error)
		return
	}
	fmt.Println("Status Code: ", httpResp.StatusCode)
	if httpResp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", httpResp.StatusCode)
	}
	a := httpResp.getNodeValue("timestamp")
	b := httpResp.getNodeValue("message")
	fmt.Println("Timestamp: ", a)
	fmt.Println("message: ", b)

}

func (r *HttpResp) getNodeValue(path string) interface{} {
	if r.JsonObj == nil {
		return nil
	}
	node, err := r.GetNode(path)
	if err != nil {
		return nil
	}
	var value interface{}
	switch node.TypeSafe() {
	case ast.V_STRING:
		value, _ = node.String()
		return value

	case ast.V_NUMBER:
		value, _ = node.Int64()
		return value
	case ast.V_FALSE, ast.V_TRUE:
		value, _ = node.Bool()
		return value
	case ast.V_ARRAY:
		arr, _ := node.Array()
		return arr
	}
	return nil

}

// 询价下单流程
func TestOrder(t *testing.T) {
	orderFlow := []RequestOptions{
		{
			Method: "GET",
			URL:    "172.19.16.220:6678/gaode/etravel/estimate/price",
			Query: map[string]interface{}{
				"channel":        "amap",
				"timestamp":      "1540446422",
				"slon":           "116.354499",
				"slat":           "39.950874",
				"sname":          "金运大厦",
				"dlon":           "116.577603",
				"dlat":           "39.908413",
				"dname":          "北京体育大学",
				"service_id":     "1",
				"product_type":   "",
				"ride_type":      "",
				"city_code":      "010",
				"departure_time": "1652694679",
				"sign":           "3B8ECEBA9D95DAC26C2367E2AC717EAF",
				"flight_no":      "CA1420",
				"flight_date":    "2021-03-28",
				"delay_time":     "1800",
				"aircode":        "PEK",
				"map_type":       "2",
			},

			Headers: map[string]string{
				"Sign-Mark":    "2",
				"Access-Token": "rh8fmsyanUJsTb1XNIym4jXHPWDys7lHDnsD1i6jNvvphSt4kZ3Px4J0Mcg5",
			},
			Timeout: 20 * time.Second,
		},
	}

	for _, opt := range orderFlow {
		resp := Do(opt)
		if resp.Error != nil {
			t.Errorf("Error making request: %v", resp.Error)
			return
		}
		fmt.Println("Status Code: ", resp.StatusCode)
		if resp.StatusCode != 200 {
			t.Errorf("Expected status code 200, got %d", resp.StatusCode)
		}
	}

}
