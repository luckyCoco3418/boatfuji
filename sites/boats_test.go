package sites

import (
	"log"
	"testing"

	"../api"
)

func TestBoats(t *testing.T) {
	// site := &Boatsetter{StoreData: true, WriteSQL: false}
	site := &Boats{StoreData: false, WriteSQL: false}
	if site.StoreData {
		api.Start()
	}
	if site.WriteSQL {
		startSQL()
	}
	// err := site.Harvest("")
	err := site.Harvest("https://www.boats.com/")
	// err := site.Harvest("https://www.boats.com/boats/ocean-alexander/45-divergence-coupe-7586270/")
	if err != nil {
		log.Printf(err.Error())
	}
	if site.WriteSQL {
		finishSQL()
	}
}
