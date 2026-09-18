/**
 * Common KRA eTIMS UNSPSC category mappings and standard classification codes.
 *
 * Merchants can either pick a broad intuitive category to auto-fill the classification
 * code, or search the comprehensive list of KRA-recognized UNSPSC codes.
 */

export type CategoryPreset = {
  id: string;
  nameEn: string;
  nameSw: string;
  code: string;
  defaultTaxCategory: "A" | "B" | "C" | "D" | "E";
  defaultUnit: string;
};

export type EtimsClassCodeOption = {
  code: string;
  nameEn: string;
  nameSw: string;
  category: string;
  defaultTaxCategory: "A" | "B" | "C" | "D" | "E";
};

export type KraUnitOfMeasure = {
  code: string;
  label: string;
};

/**
 * Standard KRA eTIMS Units of Measure (qtyUnitCd).
 * Default is PCS (Pieces / Each, mapped as PCS / U).
 */
export const KRA_UNITS_OF_MEASURE: KraUnitOfMeasure[] = [
  { code: "PCS", label: "PCS · Pieces / Items (Standard)" },
  { code: "KGM", label: "KGM / KG · Kilograms" },
  { code: "LTR", label: "LTR · Litres" },
  { code: "MTR", label: "MTR · Metres" },
  { code: "BOX", label: "BOX · Boxes" },
  { code: "PK", label: "PK · Packets / Packages" },
  { code: "SET", label: "SET · Sets" },
  { code: "CAN", label: "CAN · Cans / Tins" },
  { code: "BTL", label: "BTL · Bottles" },
  { code: "BAG", label: "BAG · Bags / Sacks" },
  { code: "CTN", label: "CTN · Cartons" },
  { code: "HRS", label: "HRS · Hours (Labour / Service)" },
  { code: "DAY", label: "DAY · Days" },
  { code: "M2", label: "M2 · Square Metres" },
  { code: "M3", label: "M3 · Cubic Metres" },
  { code: "TNE", label: "TNE · Tonnes (Metric)" },
];

/**
 * Quick-select general categories for Kenyan merchants.
 */
export const CATEGORY_PRESETS: CategoryPreset[] = [
  {
    id: "general_retail",
    nameEn: "General Retail & Groceries",
    nameSw: "Duka la Jumla na Vyakula",
    code: "50000000",
    defaultTaxCategory: "B",
    defaultUnit: "PCS",
  },
  {
    id: "food_beverages",
    nameEn: "Food & Prepared Meals (Restaurant/Cafe)",
    nameSw: "Chakula na Vinywaji (Mkahawa/Hoteli)",
    code: "50202300",
    defaultTaxCategory: "B",
    defaultUnit: "PCS",
  },
  {
    id: "exempt_staples",
    nameEn: "Unprocessed Agricultural Produce (Exempt)",
    nameSw: "Mazao ya Kilimo Yasiposindikwa (Bila Kodi)",
    code: "50101500",
    defaultTaxCategory: "A",
    defaultUnit: "KGM",
  },
  {
    id: "clothing_apparel",
    nameEn: "Clothing, Shoes & Apparel",
    nameSw: "Nguo, Viatu na Mavazi",
    code: "53100000",
    defaultTaxCategory: "B",
    defaultUnit: "PCS",
  },
  {
    id: "hardware_building",
    nameEn: "Hardware, Building & Construction",
    nameSw: "Vifaa vya Ujenzi na Hardware",
    code: "30100000",
    defaultTaxCategory: "B",
    defaultUnit: "PCS",
  },
  {
    id: "electronics_phones",
    nameEn: "Electronics, Phones & Accessories",
    nameSw: "Elektroniki, Simu na Vifaa Vyake",
    code: "43190000",
    defaultTaxCategory: "B",
    defaultUnit: "PCS",
  },
  {
    id: "professional_services",
    nameEn: "Professional & Consulting Services",
    nameSw: "Huduma za Kitaalamu na Ushauri",
    code: "80100000",
    defaultTaxCategory: "B",
    defaultUnit: "HRS",
  },
  {
    id: "transport_logistics",
    nameEn: "Transport & Logistics Services",
    nameSw: "Usafirishaji na Uchukuzi",
    code: "78100000",
    defaultTaxCategory: "B",
    defaultUnit: "PCS",
  },
  {
    id: "health_pharmacy",
    nameEn: "Medicines & Health Supplies (Exempt)",
    nameSw: "Dawa na Vifaa vya Matibabu (Bila Kodi)",
    code: "51000000",
    defaultTaxCategory: "A",
    defaultUnit: "PCS",
  },
  {
    id: "beauty_cosmetics",
    nameEn: "Beauty, Hair & Salon Services",
    nameSw: "Urembo, Saluni na Vipodozi",
    code: "53130000",
    defaultTaxCategory: "B",
    defaultUnit: "PCS",
  },
  {
    id: "stationery_printing",
    nameEn: "Stationery, Paper & Printing",
    nameSw: "Vifaa vya Ofisi na Uchapishaji",
    code: "14110000",
    defaultTaxCategory: "B",
    defaultUnit: "PCS",
  },
  {
    id: "repair_maintenance",
    nameEn: "Repair & Maintenance Services",
    nameSw: "Huduma za Ukarabati na Matengenezo",
    code: "72100000",
    defaultTaxCategory: "B",
    defaultUnit: "HRS",
  },
];

/**
 * Descriptive searchable eTIMS classification codes.
 */
export const ETIMS_CLASS_CODES: EtimsClassCodeOption[] = [
  {
    code: "50000000",
    nameEn: "Food, Beverage & Tobacco Products (General)",
    nameSw: "Vyakula na Vinywaji (Jumla)",
    category: "Groceries & FMCG",
    defaultTaxCategory: "B",
  },
  {
    code: "50101500",
    nameEn: "Fresh Fruits and Vegetables (Exempt Produce)",
    nameSw: "Matunda na Mboga Safi (Bila Kodi)",
    category: "Groceries & FMCG",
    defaultTaxCategory: "A",
  },
  {
    code: "50181900",
    nameEn: "Bread, Bakery & Confectionery",
    nameSw: "Mikate na Bidhaa za Bakery",
    category: "Groceries & FMCG",
    defaultTaxCategory: "B",
  },
  {
    code: "50202300",
    nameEn: "Non-Alcoholic Beverages & Soft Drinks",
    nameSw: "Vinywaji Baridi na Soda",
    category: "Groceries & FMCG",
    defaultTaxCategory: "B",
  },
  {
    code: "50202200",
    nameEn: "Alcoholic Beverages (Beer, Wine, Spirits)",
    nameSw: "Vinywaji vya Pombe (Bia, Mvinyo)",
    category: "Groceries & FMCG",
    defaultTaxCategory: "B",
  },
  {
    code: "53100000",
    nameEn: "Clothing, Garments & Outerwear",
    nameSw: "Nguo na Mavazi",
    category: "Fashion & Apparel",
    defaultTaxCategory: "B",
  },
  {
    code: "53110000",
    nameEn: "Footwear & Shoes",
    nameSw: "Viatu",
    category: "Fashion & Apparel",
    defaultTaxCategory: "B",
  },
  {
    code: "53130000",
    nameEn: "Personal Care Products & Cosmetics",
    nameSw: "Vipodozi na Vifaa vya Usafi Binafsi",
    category: "Beauty & Personal Care",
    defaultTaxCategory: "B",
  },
  {
    code: "43190000",
    nameEn: "Mobile Phones & Communications Devices",
    nameSw: "Simu za Mkononi na Vifaa Vyake",
    category: "Electronics",
    defaultTaxCategory: "B",
  },
  {
    code: "43210000",
    nameEn: "Computer Equipment, Laptops & Accessories",
    nameSw: "Kompyuta, Laptop na Vifaa Vyake",
    category: "Electronics",
    defaultTaxCategory: "B",
  },
  {
    code: "52140000",
    nameEn: "Domestic & Kitchen Appliances",
    nameSw: "Vyombo na Vifaa vya Nyumbani",
    category: "Home & Electronics",
    defaultTaxCategory: "B",
  },
  {
    code: "30100000",
    nameEn: "Structural Materials, Cement & Timber",
    nameSw: "Saruji, Mbao na Vifaa vya Ujenzi",
    category: "Hardware & Construction",
    defaultTaxCategory: "B",
  },
  {
    code: "27110000",
    nameEn: "Hand Tools & Hardware Goods",
    nameSw: "Zana za Kazi na Vifaa vya Hardware",
    category: "Hardware & Construction",
    defaultTaxCategory: "B",
  },
  {
    code: "25170000",
    nameEn: "Motor Vehicle Parts, Tyres & Spares",
    nameSw: "Vipuri vya Magari na Matairi",
    category: "Automotive",
    defaultTaxCategory: "B",
  },
  {
    code: "15100000",
    nameEn: "Fuels, Petrol, Diesel & Kerosene",
    nameSw: "Mafuta, Petroli na Dizeli",
    category: "Energy & Fuel",
    defaultTaxCategory: "E",
  },
  {
    code: "14110000",
    nameEn: "Paper Products, Books & Stationery",
    nameSw: "Karatasi, Vitabu na Vifaa vya Shule",
    category: "Office & Stationery",
    defaultTaxCategory: "B",
  },
  {
    code: "51000000",
    nameEn: "Pharmaceuticals & Medicines (Exempt)",
    nameSw: "Dawa za Hospitali na Matibabu",
    category: "Healthcare",
    defaultTaxCategory: "A",
  },
  {
    code: "80100000",
    nameEn: "Management Advisory & Consulting Services",
    nameSw: "Huduma za Ushauri wa Biashara",
    category: "Services",
    defaultTaxCategory: "B",
  },
  {
    code: "81160000",
    nameEn: "Information Technology & Software Services",
    nameSw: "Huduma za Teknolojia na Programu za Kompyuta",
    category: "Services",
    defaultTaxCategory: "B",
  },
  {
    code: "78100000",
    nameEn: "Transport, Courier & Delivery Services",
    nameSw: "Huduma za Usafiri na Uwasilishaji",
    category: "Services",
    defaultTaxCategory: "B",
  },
  {
    code: "72100000",
    nameEn: "Maintenance, Construction & Repair Services",
    nameSw: "Huduma za Ukarabati na Ujenzi",
    category: "Services",
    defaultTaxCategory: "B",
  },
  {
    code: "90100000",
    nameEn: "Restaurant, Catering & Meal Services",
    nameSw: "Huduma za Mkahawa na Upishi",
    category: "Services",
    defaultTaxCategory: "B",
  },
  {
    code: "99010000",
    nameEn: "General Goods (VAT Act Standard Rate)",
    nameSw: "Bidhaa za Kawaida (Kiwango cha Kawaida cha VAT)",
    category: "General / Other",
    defaultTaxCategory: "B",
  },
  {
    code: "99011000",
    nameEn: "General Goods (VAT Act Exempt Rate)",
    nameSw: "Bidhaa za Kawaida (Zilizosamehewa Kodi)",
    category: "General / Other",
    defaultTaxCategory: "A",
  },
  {
    code: "99020000",
    nameEn: "General Services (VAT Act Standard Rate)",
    nameSw: "Huduma za Kawaida (Kiwango cha Kawaida cha VAT)",
    category: "General / Other",
    defaultTaxCategory: "B",
  },
];
