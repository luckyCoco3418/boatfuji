package sites

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"../api"
)

func init() {
	boats := &Boats{}
	boats.init()

	api.Sites[boats.bsBaseURL] = boats
}

// Boats accesses https://www.boatsetter.com/
type Boats struct {
	StoreData bool
	WriteSQL  bool

	bsBaseURL          string
	bsBaseDir          string
	bsURLPattern       *regexp.Regexp
	bsURL2ModelPattern *regexp.Regexp
	bsLengthPatter     *regexp.Regexp

	// bsUserMap map[string]int64 // from id like "abcdefg" to UserID
	bsBoatMap map[string]int64 // from id like "abcdefg" to BoatID

	boatsByFilters map[string][]string

	nextDealID int64
}

func (site *Boats) init() {
	site.bsBaseURL = "https://www.boats.com/"
	site.bsBaseDir = "harvest/www.boats.com/"
	site.bsURLPattern = regexp.MustCompile(`^https://www\.boats\.com/(boats|power-boats|sailing-boats|unpowered)/.*-(\w+)\/$`)
	site.bsURL2ModelPattern = regexp.MustCompile(`^https://www\.boats\.com/(boats)/(.*)/(.*)-\w+\/$`)
	site.bsLengthPatter = regexp.MustCompile(`([0-9]+) ft( ([0-9]+) in)?`)

	site.bsBoatMap = map[string]int64{}

	site.nextDealID = 1
}

// Harvest gets data from the site
func (site *Boats) Harvest(url string) error {
	boatsByFilters = map[string][]string{}
	filtersJSON, err := ioutil.ReadFile(site.bsBaseDir + "filters.json")
	if err == nil {
		err = json.Unmarshal(filtersJSON, &boatsByFilters)
		if err != nil {
			return err
		}
	}
	os.MkdirAll(site.bsBaseDir+"boats-for-sale", 0755)
	os.MkdirAll(site.bsBaseDir+"boats", 0755)
	os.MkdirAll(site.bsBaseDir+"urls", 0755) // store urls of boat
	os.MkdirAll("www/i", 0755)               // store images of boat
	// os.MkdirAll(site.bsBaseDir+"users", 0755)
	if url == "" {
		// loop through users and boats folders
		for _, dir := range []string{"boats" /*"users"*/} {
			files, err := ioutil.ReadDir(site.bsBaseDir + dir)
			if err != nil {
				return err
			}
			oldPercent := ""
			for fileIndex, file := range files {
				newPercent := strconv.Itoa(100 * fileIndex / len(files))
				if newPercent != oldPercent {
					log.Println(dir + " " + newPercent + "%")
				}
				oldPercent = newPercent
				var fileName = file.Name()
				if strings.HasSuffix(fileName, ".htm") {
					id := strings.Split(filepath.Base(fileName), ".")[0]
					if _, err = site.harvestX("", dir, id); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if url == site.bsBaseURL {
		boatsByFilters = map[string][]string{}
		for _, filter := range []string{
			"&boat-type=power", "&boat-type=sail", "&boat-type=unpowered",
			"&activity=overnight-cruising", "&activity=day-cruising", "&activity=watersports", "&activity=freshwater-fishing", "&activity=saltwater-fishing", "&activity=sailing", "&activity=pwc", "",
		} {
			// TEMP for page := 1; page < 99999; page++ {
			for page := 1; page < 10; page++ {
				boatsPage, err := getPage(site.bsBaseDir+"boats-for-sale/"+strconv.Itoa(page)+filter+".htm", "https://www.boats.com/boats-for-sale/?page="+strconv.Itoa(page)+filter)
				if err != nil {
					return err
				}
				if boatsPage.Find1(nil, "//strong[ends-with(text(),'Boats Available')]", "0", "0") == "0" {
					break
				}
				bsBoatIDs := boatsPage.FindN(nil, "//li/@data-reporting-impression-product-id", 0, 99999, "", "")
				if filter != "" {
					filtersKey := strings.Split(filter, "=")[1]
					if page == 1 {
						boatsByFilters[filtersKey] = bsBoatIDs
					} else {
						boatsByFilters[filtersKey] = append(boatsByFilters[filtersKey], bsBoatIDs...)
					}
					continue
				}
				/*
					for _, bsBoatID := range bsBoatIDs {
						_, err = harvestBoat(bsBoatID, 0)
						if err != nil {
							return err
						}
					}
				*/
			}
		}
		filtersJSON, _ := json.Marshal(boatsByFilters)
		ioutil.WriteFile(site.bsBaseDir+"filters.json", filtersJSON, 0644)
		return nil
	}
	match := site.bsURLPattern.FindStringSubmatch(url)
	if match == nil {
		return errors.New("BadURL")
	}
	_, err = site.harvestX(url, match[1], match[2])
	return err
}

func (site *Boats) harvestX(url, x, id string) (int64, error) {
	return site.harvestBoat(url, x, id, 0, nil)
}

// var bsAnalyticsPattern = regexp.MustCompile(`<script>\n +analytics\.identify\("[^"]+", \{"initial_page_route":"/(boat-rentals|boats/\w+)"\}\);\n</script>`)
// var bsAvgResponseTimePattern = regexp.MustCompile(`>\nAvg\. response time\n<span class='[^']*'>(?:N/A|(&[lg]t;) (\d+) (min|hour))`)
// var bsResponseRatePattern = regexp.MustCompile(`>\nResponse rate\n<div class='[^']*'>(?:N/A|(\d+)%)`)
// var bsBoatPassengersPattern = regexp.MustCompile(`^ *Up to (\d+) people$`)
// var bsBoatHorsepowerPattern = regexp.MustCompile(`^(\d+) hp$`)
// var bsBoatCityPattern = regexp.MustCompile(`^([^,]*), (\w{2})?$`)
// var bsBoatLocationPattern = regexp.MustCompile(`var evergage_boatLatitude = "([^"]+)";\n *var evergage_boatLongitude = "([^"]+)";`)
// var bsBoatPackagesPattern = regexp.MustCompile(`packages: (.*?),\n`)
var bsTankCapacityPattern = regexp.MustCompile(`^([\d.]+) gal`)
var bsLbWeightPattern = regexp.MustCompile(`^([\d.]+) lb`)

// type bsPackage struct {
// 	ID              int       `json:"id"`
// 	PublicID        string    `json:"public_id"`
// 	IsDefault       bool      `json:"is_default"`
// 	Type            string    `json:"type" enum:"bareboat, captained"`
// 	InstantBookable bool      `json:"instant_bookable"`
// 	IsCharter       bool      `json:"is_charter"`
// 	OwnerInsured    bool      `json:"owner_provides_insurance"`
// 	Prices          []bsPrice `json:"prices"`
// }

// type bsPrice struct {
// 	BoatPrice           string           `json:"boat_price"`
// 	ServiceFee          string           `json:"service_fee"`
// 	TowingFee           string           `json:"towing_fee"`
// 	BoatingCreditsValue string           `json:"boating_credits_value"`
// 	CouponValue         string           `json:"coupon_value"`
// 	SalesTax            string           `json:"sales_tax"`
// 	TotalPrice          string           `json:"total_price"`
// 	SecurityDeposit     string           `json:"security_deposit"`
// 	CalendarDays        int              `json:"calendar_days"`
// 	RangeAvailable      bool             `json:"range_available"`
// 	DatePriceAdjustment string           `json:"date_price_adjustment"`
// 	CaptainPrice        string           `json:"captain_price"`
// 	RegularPrice        string           `json:"regular_price"`
// 	CaptainFee          string           `json:"captain_fee"`
// 	Value               string           `json:"value"`
// 	IsDefault           bool             `json:"is_default"`
// 	Duration            string           `json:"duration" enum:"all_day, half_day"`
// 	FuelPolicy          string           `json:"fuel_policy" enum:"owner_pays, renter_pays"`
// 	SpecialPrices       []bsSpecialPrice `json:"special_prices"`
// }

// type bsSpecialPrice struct {
// }

func (site *Boats) FindModelInURL(url string) (make string, model string, err error) {
	match := site.bsURL2ModelPattern.FindStringSubmatch(url)
	if match == nil {
		return "", "", errors.New("BadURL for Make/Model")
	}
	make = strings.Title(strings.ReplaceAll(match[2], "-", " "))
	model = strings.Title(strings.ReplaceAll(match[3], "-", " "))
	return make, model, nil
}

func Feet(s string, re *regexp.Regexp) (float64, error) {
	feetStr := s
	inchStr := "0"
	if re != nil {
		match := re.FindStringSubmatch(s)
		if match == nil {
			err := fmt.Errorf("NeedFeetString \"%s\" %s", s, re.String())
			return 0, err
		}
		if len(match) > 1 {
			feetStr = match[1]
		}
		if len(match) > 3 {
			inchStr = match[3]
		}
	}
	feet, err := strconv.ParseFloat(feetStr, 64)
	if err != nil {
		err = fmt.Errorf("NeedFloat64 \"%s\"", feetStr)
		return 0, err
	}
	inch, err := strconv.ParseFloat(inchStr, 64)
	if err != nil {
		err = fmt.Errorf("NeedFloat64 \"%s\"", inchStr)
		inch = 0
	}
	return Round(feet+(inch/12), 6), err
}

func Round(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(math.Round(num*output)) / output
}

func (site *Boats) getLength(boatPage *page, expr string) float64 {
	lengthStr := boatPage.Find1(nil, expr, "", "")
	if lengthStr == "" {
		return 0
	}
	length, err := Feet(lengthStr, site.bsLengthPatter)
	if err != nil {
		boatPage.Warn(err.Error())
	}
	return length
}

// harvestBoat will harvest the given boat (i.e., id=abcdefg), either from the harvest folder or from the website
func (site *Boats) harvestBoat(url, x, bsBoatID string, userIDIfUnavailable int64, onlyGetUserInfo *api.User) (int64, error) {
	if boatID, ok := site.bsBoatMap[bsBoatID]; ok && onlyGetUserInfo == nil {
		// commentSQL("cached boat " + bsBoatID)
		return boatID, nil
	}
	// commentSQL("starting boat " + bsBoatID)

	filePath := site.bsBaseDir + "urls/" + bsBoatID + ".txt"
	if url == "" {
		fileData, err := ioutil.ReadFile(filePath)
		if err != nil {
			return 0, err
		}
		url = string(fileData)
	} else {
		// store boat's url
		ioutil.WriteFile(filePath, []byte(url), 0644)
	}

	boat := api.Boat{URLs: []string{url}, Sale: &api.BoatSale{}}
	boatFileWithoutExt := site.bsBaseDir + "boats/" + bsBoatID
	boatPage, err := getPage(boatFileWithoutExt+".htm", url)
	if err != nil {
		return 0, err
	}

	// see if this boat wasn't availble so it redirected to boat-rentals
	available := true //:= boatPage.Find1ByRE(bsAnalyticsPattern, 1, "boat-rentals", "boat-rentals") != "boat-rentals"
	// if boatPage.Find0or1(nil, "//h1[text()='BLIMEY!']", "", "") == "BLIMEY!" {
	// 	available = false
	// }
	// userInfo := onlyGetUserInfo
	// if userInfo == nil {
	// 	userInfo = &api.User{}
	// }
	// if available {
	// 	// get user response rate and time
	// 	userInfo.RequestCount = 1000
	// 	userInfo.ResponseCount = 10 * boatPage.Int(changeIf("", "0", boatPage.Find1ByRE(bsResponseRatePattern, 1, "0", "0")), nil)
	// 	avgResponseSecs, _ := strconv.Atoi(changeIf("", "0", changeIf("min", "60", changeIf("hour", "3600", boatPage.Find1ByRE(bsAvgResponseTimePattern, 3, "", "")))))
	// 	if avgResponseSecs > 0 {
	// 		avgResponseSecs *= boatPage.Int(boatPage.Find1ByRE(bsAvgResponseTimePattern, 2, "", ""), nil)
	// 		if boatPage.Find1ByRE(bsAvgResponseTimePattern, 1, "", "") == "<" {
	// 			avgResponseSecs--
	// 		} else {
	// 			avgResponseSecs++
	// 		}
	// 	}
	// 	userInfo.ResponseSecSum = 1000 * avgResponseSecs
	// }
	// if onlyGetUserInfo != nil {
	// 	return 0, nil
	// }

	// get UserID
	if !available {
		// it redirected to a boats list because boat is not currently available
		boat.UserID = userIDIfUnavailable
	} else {
		// bsUserID := strings.TrimPrefix(boatPage.Find1(nil, "//a[text()='View profile']/@href", "", ""), "/users/")
		// userID, err := site.harvestUser(bsUserID, true, userInfo)
		// if err != nil {
		// 	return 0, err
		// }
		// boat.UserID = userID
		boat.UserID = userIDIfUnavailable

		// get boat info
		fieldXPath := func(x, name string) string {
			if x == "boats" {
				return `//div[@class='description-list__row'][dt[text()='` + name + `']]/dd[@class='description-list__description']`
			}
			return `//tr[th[contains(text(),'` + name + `')]]/td`
		}
		boat.Year = boatPage.Int(boatPage.Find1(nil, fieldXPath(x, "Year"), "0", "0"), nil)
		boat.Make = boatPage.Find0or1(nil, fieldXPath(x, "Make"), "", "")
		boat.Model = boatPage.Find0or1(nil, fieldXPath(x, "Model"), "", "")
		if boat.Make == "" {
			make, model, err := site.FindModelInURL(url)
			if err != nil {
				boatPage.Warn("BadMake \"" + url + "\"")
				return 0, nil
			}
			boat.Make = make
			boat.Model = model
		}

		make := api.LookupMake(boat.Year, 0, 0, boat.Make) // TODO
		if make != nil {
			boat.MakeID = make.ID
			for _, detail := range make.Details {
				boat.Type = detail.Type
				break
			}
		}
		category := boatPage.Find1(nil, fieldXPath(x, "Class"), "", "")
		if category != "" {
			if category == "Kayak" {
				category = "Canoe/Kayak"
			}
			_, cats := api.Enums(api.Boat{}, "Category")
			for code, label := range cats {
				if strings.ToUpper(category) == strings.ToUpper(label) {
					boat.Category = code
				}
			}
			if boat.Category == "" {
				boatPage.Warn("BadCategory \"" + category + "\"")
			}
		}
		condition := boatPage.Find1(nil, fieldXPath(x, "Condition"), "", "")
		if condition != "" {
			_, cond := api.Enums(api.Boat{}, "Condition")
			for code, label := range cond {
				if strings.ToUpper(condition) == strings.ToUpper(label) {
					boat.Condition = code
					break
				}
			}
			if boat.Condition == "" {
				boatPage.Warn("BadCondition \"" + condition + "\"")
			}
		}

		boat.Length = float32(site.getLength(boatPage, fieldXPath(x, "Length")))
		passengers := boatPage.Find0or1(nil, fieldXPath(x, "Max Passengers"), "", "")
		if passengers != "" {
			boat.Passengers = boatPage.Int(passengers, nil)
		}
		// boat.Sleeps = boatPage.Int(boatPage.Find0or1(nil, fieldXPath("Sleeps"), "0", "0"), nil)
		// boat.Rooms = boatPage.Int(boatPage.Find0or1(nil, fieldXPath("Staterooms"), "0", "0"), nil)
		if api.StringInArray(bsBoatID, boatsByFilters["power"]) {
			boat.Locomotion = "Power"
		} else if api.StringInArray(bsBoatID, boatsByFilters["sail"]) {
			boat.Locomotion = "Sail"
		} else if api.StringInArray(bsBoatID, boatsByFilters["unpowered"]) {
			boat.Locomotion = "Unpowered"
		} else {
			boat.Locomotion = "Power"
			// boatPage.Warn("BadLocomotion")
		}
		boat.HullMaterials = boatPage.FindN(nil, fieldXPath(x, "Hull Material"), 0, 99, "", "")
		boat.Beam = float32(site.getLength(boatPage, fieldXPath(x, "Beam")))
		boat.Draft = float32(site.getLength(boatPage, fieldXPath(x, "Max Draft")))
		boat.Weight = float32(boatPage.Float64(boatPage.Find0or1(nil, fieldXPath(x, "Dry Weight"), "0 lb", "0 lb"), bsLbWeightPattern))
		boat.BridgeClearance = float32(site.getLength(boatPage, fieldXPath(x, "Max Bridge Clearance")))

		enginePowers := boatPage.FindN(nil, fieldXPath(x, "Power"), 0, 99, "", "")
		if enginePowers[0] != "" {
			boat.EnginePower = boatPage.Int(enginePowers[0], bsBoatHorsepowerPattern)
		}
		boat.EngineCount = boatPage.Int(changeIf("", "0", boatPage.Find0or1(nil, fieldXPath(x, "Number of Engines"), "0", "0")), nil)
		if (boat.EngineCount == 0) && (enginePowers[0] != "") {
			boat.EngineCount = len(enginePowers)
		}
		engineMakes := boatPage.FindN(nil, fieldXPath(x, "Engine Make"), 0, 99, "", "")
		boat.EngineMake = engineMakes[0]
		engineModels := boatPage.FindN(nil, fieldXPath(x, "Engine Model"), 0, 99, "", "")
		boat.EngineModel = engineModels[0]

		fuleTypes := boatPage.FindN(nil, fieldXPath(x, "Fuel Type"), 0, 99, "", "")
		boat.FuelType = fuleTypes[0]
		boat.FuelCapacity = float32(boatPage.Float64(strings.Trim(boatPage.Find0or1(nil, fieldXPath(x, "Fuel Tanks"), "0 gal", "0 gal"), " \n"), bsTankCapacityPattern))
		boat.FreshWaterCapacity = float32(boatPage.Float64(strings.Trim(boatPage.Find0or1(nil, fieldXPath(x, "Fresh Water Tanks"), "0 gal", "0 gal"), " \n"), bsTankCapacityPattern))
		boat.GrayWaterCapacity = float32(boatPage.Float64(strings.Trim(boatPage.Find0or1(nil, fieldXPath(x, "Holding Tanks"), "0 gal", "0 gal"), " \n"), bsTankCapacityPattern))

		description := ""
		loa := float32(site.getLength(boatPage, fieldXPath(x, "LOA")))
		if loa > 0 {
			description = description + fmt.Sprintf("%s:%f\n", "LOA", loa)
		}
		lwl := float32(site.getLength(boatPage, fieldXPath(x, "Length at Water Line")))
		if lwl > 0 {
			description = description + fmt.Sprintf("%s:%f\n", "Length at Water Line", lwl)
		}
		dat := boatPage.Find0or1(nil, fieldXPath(x, "Deadrise at Transom"), "", "")
		if dat != "" {
			description = description + fmt.Sprintf("%s:%s\n", "Deadrise at Transom", dat)
		}
		ma := boatPage.Find0or1(nil, fieldXPath(x, "Mainsail Area"), "", "")
		if ma != "" {
			description = description + fmt.Sprintf("%s:%s\n", "Mainsail Area", ma)
		}
		engineTypes := boatPage.FindN(nil, fieldXPath(x, "Engine Type"), 0, 99, "", "")
		if engineTypes[0] != "" {
			description = description + fmt.Sprintf("%s:%s\n", "Engine Type", engineTypes[0])
		}
		hs := boatPage.Find0or1(nil, fieldXPath(x, "Hull Shape"), "", "")
		if hs != "" {
			description = description + fmt.Sprintf("%s:%s\n", "Hull Shape", hs)
		}
		lifeStyle := boatPage.Find0or1(nil, fieldXPath(x, "Lifestyle"), "", "")
		if lifeStyle != "" {
			description = description + fmt.Sprintf("%s:%s\n", "Lifestyle", lifeStyle)
		}
		boat.Sale.ListingDescription = description

		location := boatPage.Find0or1(nil, fieldXPath(x, "Location"), "", "")
		if location != "" {
			boat.Location = &api.Contact{
				Type:  "Address",
				Line1: location,
			}
		}
		// keyInfos := boatPage.FindN(nil, `//h3[@class='u-fsBase u-textSemiBold']`, 3, 4, "", "")
		// for keyInfoIndex, keyInfo := range keyInfos {
		// 	// usually it will be citystate, optionally instant bookable, captain, passengers
		// 	if keyInfoIndex == 0 {
		// 		match := bsBoatCityPattern.FindStringSubmatch(keyInfo)
		// 		if match != nil {
		// 			boat.Location.City = match[1]
		// 			boat.Location.State = match[2]
		// 			boat.Location.Country = "US"
		// 		} else {
		// 			// TODO: call Google API for something like "Cancún, QROO"
		// 			// TODO: boatPage.Warn("BadCityState \"" + keyInfo + "\"")
		// 		}
		// 	} else if keyInfo == "Instant bookable" {
		// 		boat.Rental.InstantBook = true
		// 	}
		// }
		// features := boatPage.FindN(nil, "//div[@data-remodal-id='js-modal-features']/div/div/div[@class='u-textRegular']", 0, 99999, "", "")
		// if features[0] != "" {
		// 	boat.Amenities = []string{}
		// 	_, amenities := api.Enums(api.Boat{}, "Amenities")
		// 	for _, feature := range features {
		// 		found := false
		// 		for code, label := range amenities {
		// 			if strings.ToUpper(feature) == strings.ToUpper(label) {
		// 				boat.Amenities = append(boat.Amenities, code)
		// 				found = true
		// 			}
		// 		}
		// 		if !found {
		// 			boatPage.Warn("BadAmenity: " + feature)
		// 		}
		// 	}
		// 	sort.Strings(boat.Amenities)
		// }
		boat.Activities = []string{}
		for _, filter := range []string{"overnight-cruising", "day-cruising", "watersports", "freshwater-fishing", "saltwater-fishing", "sailing", "pwc"} {
			if api.StringInArray(bsBoatID, boatsByFilters[filter]) {
				var activity string
				if strings.HasSuffix(filter, "cruising") {
					activity = "Cruising"
				} else if strings.HasSuffix(filter, "fishing") {
					activity = "Fishing"
				} else if strings.EqualFold(filter, "pwc") {
					activity = "PWC"
				} else {
					activity = strings.Title(filter)
				}
				boat.Activities = append(boat.Activities, activity)
			}
		}
		sort.Strings(boat.Activities)
		// get boat images
		imageListNodes := boatPage.FindNodes("//div[@class='carousel'][//ul[@class='main']]")
		if len(imageListNodes) > 0 {
			imageURLs := boatPage.FindN(imageListNodes[0], "//li/@data-src_w0", 0, 99999, "", "")
			images := []api.Image{}
			for _, imageURL := range imageURLs {
				if imageURL != "" {
					image := boatPage.Image(imageURL, 600, 400, nil)
					if image != nil {
						images = append(images, *image)
					}
				}
			}
			if len(images) > 0 {
				boat.Images = images
			}
		}
		// // get rental info
		// boat.Rental.ListingTitle = boatPage.Find1(nil, "//h1", "", "")
		// boat.Rental.ListingDescription = strings.TrimSpace(boatPage.Find1(nil, "//div[@class='u-mb1 js-show-more-content']", "", ""))
		// boat.Rental.ListingSummary = "" // TODO
		// boat.Rental.Rules = ""          // TODO
		// boat.Rental.CancelPolicy = boatPage.Find0or1(nil, "//div[h3[text()='Cancellation policy']]/div", "Moderate", "Moderate")
		// // count reviews and stars
		// reviewStars := boatPage.FindN(nil, "//div[@data-remodal-id='js-modal-reviews']//span[@class='u-hiddenVisually']", 0, 99999, "", "")
		// for _, stars := range reviewStars {
		// 	if strings.HasSuffix(stars, "/5 stars") {
		// 		rating, _ := strconv.Atoi(strings.TrimSuffix(stars, "/5 stars"))
		// 		boat.Rental.ReviewCount++
		// 		boat.Rental.ReviewRatingSum += rating
		// 	} else if stars != "" {
		// 		boatPage.Warn("BadReviewStars " + stars)
		// 	}
		// }

		// // get boat packages and pricing
		// rentalPricing := []api.BoatRentalPricing{}
		// packagesJSON := boatPage.Find1ByRE(bsBoatPackagesPattern, 1, "[]", "[]")
		// packages := []bsPackage{}
		// if err := json.Unmarshal([]byte(packagesJSON), &packages); err != nil {
		// 	boatPage.Warn("BadPackages: " + packagesJSON)
		// }
		// for _, pkg := range packages {
		// 	for _, price := range pkg.Prices {
		// 		captain := "NoCaptain"
		// 		if pkg.Type == "captained" {
		// 			if price.CaptainPrice == "0.00" {
		// 				captain = "CaptainIncluded"
		// 			} else {
		// 				captain = "CaptainExtra"
		// 			}
		// 		}
		// 		rentalPricingIndex := -1
		// 		for i, item := range rentalPricing {
		// 			if item.Captain == captain {
		// 				rentalPricingIndex = i
		// 			}
		// 		}
		// 		if rentalPricingIndex == -1 {
		// 			rentalPricingIndex = len(rentalPricing)
		// 			rentalPricing = append(rentalPricing, api.BoatRentalPricing{
		// 				Captain: captain,
		// 			})
		// 		}
		// 		boatPrice, _ := strconv.ParseFloat(price.BoatPrice, 32)
		// 		switch price.Duration {
		// 		case "all_day":
		// 			rentalPricing[rentalPricingIndex].DailyPrice = float32(boatPrice)
		// 		case "half_day":
		// 			rentalPricing[rentalPricingIndex].HalfDailyPrice = float32(boatPrice)
		// 		default:
		// 			boatPage.Warn("BadDuration: " + price.Duration)
		// 		}
		// 		switch price.FuelPolicy {
		// 		case "owner", "owner pays", "owner_pays":
		// 			rentalPricing[rentalPricingIndex].FuelPayer = "owner"
		// 		case "renter", "renter pays", "renter_pays":
		// 			rentalPricing[rentalPricingIndex].FuelPayer = "renter"
		// 		default:
		// 			// boatPage.Warn("BadFuelPolicy: " + price.FuelPolicy)
		// 		}
		// 	}
		// }
		// boat.Rental.Seasons = []api.BoatRentalSeason{
		// 	{
		// 		StartDay: api.Date(2000, 1, 1),
		// 		EndDay:   api.Date(2000, 12, 31),
		// 		Pricing:  rentalPricing,
		// 	},
		// }
	}
	// save record and warnings
	var boatID int64
	if site.StoreData {
		resp := api.SetBoat(&api.Request{Session: &api.Session{IsGod: true}, Boat: &boat}, nil)
		boatID = resp.ID
		if boatID == 0 {
			boatPage.Warn(fmt.Sprintf("SetBoat: %v", resp))
		}
	} else {
		boatID, _ = strconv.ParseInt(bsBoatID, 10, 64)
	}
	boat.ID = boatID
	if site.WriteSQL {
		writeBoatSQL(&boat)
	}
	boatJSON, _ := json.Marshal(boat)
	ioutil.WriteFile(boatFileWithoutExt+".json", boatJSON, 0644)
	boatPage.SaveWarnings(boatFileWithoutExt + ".txt")
	site.bsBoatMap[bsBoatID] = boatID
	// commentSQL("caching boat " + bsBoatID)
	return boatID, nil
}

// var bsAboardSincePattern = regexp.MustCompile(`^Aboard since (\d{4})$`)
// var bsUserCityPattern = regexp.MustCompile(`^From ([^,]*), (\w{2})?$`)
// var bsReviewCtPattern = regexp.MustCompile(`^\n(\d+) reviews?\n$`)
// var bsDatePattern = regexp.MustCompile(`^(\w{3})\. (\d\d)(st|nd|rd|th)$`)
