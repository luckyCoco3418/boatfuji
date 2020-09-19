package sites

import (
	"log"
	"testing"

	"boatfuji.com/api"
)

func TestBoats(t *testing.T) {
	// site := &Boatsetter{StoreData: true, WriteSQL: false}
	site := &Boats{StoreData: false, WriteSQL: false}
	site.init()
	if site.StoreData {
		api.Start()
	}
	if site.WriteSQL {
		startSQL()
	}
	// err := site.Harvest("")
	// err := site.Harvest("https://www.boats.com/")

	// err := site.Harvest("https://www.boats.com/boats/ocean-alexander/45-divergence-coupe-7586270/")
	// err := site.Harvest("https://www.boats.com/power-boats/2020-ocean-alexander-45-divergence-coupe-7431757/")
	// err := site.Harvest("https://www.boats.com/power-boats/2021-yamaha-waverunner-fx-cruiser-ho-7562361/")
	// err := site.Harvest("https://www.boats.com/sailing-boats/2021-sunreef-50-7561964/")
	// err := site.Harvest("https://www.boats.com/unpowered/2020-ascend-133x-sit-on-titanium-7463469/")

	if err != nil {
		log.Printf(err.Error())
	}
	if site.WriteSQL {
		finishSQL()
	}
}
