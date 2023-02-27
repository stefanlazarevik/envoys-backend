package spot

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cryptogateway/backend-envoys/assets"
	"github.com/cryptogateway/backend-envoys/assets/common/decimal"
	"github.com/cryptogateway/backend-envoys/assets/common/help"
	"github.com/cryptogateway/backend-envoys/assets/common/query"
	"github.com/cryptogateway/backend-envoys/server/proto/pbspot"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"google.golang.org/grpc/status"
	"os"
	"path/filepath"
	"strings"
)

const (
	ExchangeType = 0
)

// Service - The purpose of the Service struct is to store data related to a service, such as the Context, run and wait maps, and
// the block map. The Context is a pointer to an assets Context, which contains information about the service. The run
// and wait maps are booleans that indicate whether the service is running or waiting for an action. The block map is an
// integer that stores the block number associated with a particular service.
type Service struct {
	Context *assets.Context

	run, wait map[int64]bool
	block     map[int64]int64
}

// Initialization - The purpose of this code is to start up multiple goroutines to run functions related to a service. In this case, the
// functions that are started are replayPriceScale(), replayMarket(), replayChainStatus(), replayDeposit(), and replayWithdraw().
func (e *Service) Initialization() {
	go e.price()
	go e.market()
	go e.chain()
	go e.deposit()
	go e.withdraw()
	go e.reward()
}

// getSecure - This function is used to get a secure string from the database, based on the user's authentication information. It
// takes the context as an argument and uses it to obtain the user's authentication information. Then it queries the
// database for the "email_code" associated with the user's account and returns it.
func (e *Service) getSecure(ctx context.Context) (secure string, err error) {

	// This code snippet is used to authenticate a user. It attempts to get the user's authentication credentials from the
	// context, and returns an error if it fails to do so. If authentication succeeds, the code will continue to execute.
	auth, err := e.Context.Auth(ctx)
	if err != nil {
		return secure, err
	}

	if err := e.Context.Db.QueryRow("select email_code from accounts where id = $1", auth).Scan(&secure); err != nil {
		return secure, err
	}

	return secure, nil
}

// setSecure - This function is used to set a secure code for a user's account. The context and a boolean parameter are passed in to
// the function to determine the action that should be taken. If the boolean is false, the function will generate a
// six-character key code and use it to migrate sample posts to the user's account. If the boolean is true, the code is
// set to an empty string. Finally, the code is stored in the user's account in the database.
func (e *Service) setSecure(ctx context.Context, cleaning bool) error {

	// This code snippet is used to authenticate a user. It attempts to get the user's authentication credentials from the
	// context, and returns an error if it fails to do so. If authentication succeeds, the code will continue to execute.
	auth, err := e.Context.Auth(ctx)
	if err != nil {
		return err
	}

	// The purpose of this code is to obtain a key code from the help package and assign it to the variable code. If an
	// error is encountered, it will return the error.
	code := help.NewCode(6, true)

	// This is a logical comparison statement. It is evaluating the boolean value of the variable "cleaning". If "cleaning"
	// is false, then the code block following the statement will execute.
	if !cleaning {

		// The purpose of the code snippet is to create a new Migrate object from the query package, and assign the Context of
		// the environment to it. This Migrates object can then be used to migrate data from one database to another.
		var (
			migrate = query.Migrate{
				Context: e.Context,
			}
		)

		// The purpose of this line of code is to email the user using the SMTP authentication credentials (auth),
		// using a secure protocol (Secure), and including a code (code) as part of the email.
		go migrate.SendMail(auth, "secure", code)

	} else {
		code = ""
	}

	if _, err = e.Context.Db.Exec("update accounts set email_code = $2 where id = $1;", auth, code); err != nil {
		return err
	}

	return nil
}

// setTrade - This function is used to set a trade in a database. It takes a series of orders (param) as an argument and performs
// various operations including inserting data into the database, calculating fees, and publishing order status and trade candles.
func (e *Service) setTrade(param ...*pbspot.Order) error {

	// This is a conditional statement that is used to check the value of the parameter at the index of 0. If the value of
	// the parameter at index 0 is equal to 0, then the function will return nil.
	if param[0].GetValue() == 0 {
		return nil
	}

	// This code is used to insert a new row of data into the trades table of a database. The values for the new row are
	// taken from the param[0] variable. If the insertion fails, an error is returned.
	if _, err := e.Context.Db.Exec(`insert into trades (assigning, base_unit, quote_unit, price, quantity) values ($1, $2, $3, $4, $5)`, param[0].GetAssigning(), param[0].GetBaseUnit(), param[0].GetQuoteUnit(), param[0].GetPrice(), param[0].GetValue()); err != nil {
		return err
	}

	// The purpose of this "for" loop is to loop through a sequence of numbers (in this case, 0 and 1) and execute a certain
	// set of instructions a certain number of times (in this case, twice).
	for i := 0; i < 2; i++ {

		// This code is used to set the Quantity of the parameter at index i in the param array. It first sets the Quantity to
		// the value of the parameter at index 0 of the array. If the GetEqual method of the parameter at index i returns true,
		// the Quantity is then set to the value of the parameter at index 1 of the array.
		param[i].Quantity = param[0].GetValue()
		if param[i].Param.GetEqual() {
			param[i].Quantity = param[1].GetValue()
		}

		// This code is checking if param[i].Param.GetMaker() is true and if it is, it is setting the price of param[i] to the
		// price of param[0]. This is likely setting the price to a predetermined value, or to a reference point to determine a price.
		param[i].Price = param[i].GetPrice()
		if param[i].Param.GetMaker() {
			param[i].Price = param[0].GetPrice()
		}

		// This code is used to insert data into the "transfers" table in a database using the parameters provided in the array
		// "param". The code first checks for any errors in the insertion process, and if there are any, it will return an error.
		if _, err := e.Context.Db.Exec(`insert into transfers (order_id, assigning, user_id, base_unit, quote_unit, price, quantity, fees) values ($1, $2, $3, $4, $5, $6, $7, $8)`, param[i].GetId(), param[i].GetAssigning(), param[i].GetUserId(), param[i].GetBaseUnit(), param[i].GetQuoteUnit(), param[i].GetPrice(), param[i].GetQuantity(), e.getFees(param[i].GetQuoteUnit(), param[i].Param.GetMaker())); err != nil {
			return err
		}

		// This statement is checking to see if the value of the parameter at index i in the param array is greater than 0. If
		// it is, then the code within the if statement will be executed. This is likely being used to check if a fee is
		// associated with the parameter at index i.
		if param[i].Param.GetFees() > 0 {

			// The purpose of this code is to determine the symbol to use in a calculation. It is using a parameter from the param
			// array to decide which symbol to use. If the parameter has a "turn" parameter set to true, the code sets the symbol
			// to the base unit, otherwise it sets the symbol to the quote unit.
			symbol := param[0].GetQuoteUnit()
			if param[i].Param.GetTurn() {
				symbol = param[0].GetBaseUnit()
			}

			// This code is updating the "fees_charges" column in the "currencies" table in a database. The "symbol" and
			// "param[i].Param.GetFees()" are parameters that are passed into the statement. If an error occurs during the
			// execution of the statement, the function will return the error.
			if _, err := e.Context.Db.Exec("update currencies set fees_charges = fees_charges + $2 where symbol = $1;", symbol, param[i].Param.GetFees()); err != nil {
				return err
			}
		}

		// The purpose of the code snippet is to publish a particular order to an exchange with the routing key "order/status".
		// The if statement checks for any errors encountered while publishing the order, and returns an error if one occurs.
		if err := e.Context.Publish(e.getOrder(param[i].GetId()), "exchange", "order/status"); err != nil {
			return err
		}
	}

	// The for loop is used to iterate through each element in the Depth() array. The underscore is used to assign the index
	// number to a variable that is not used in the loop. The interval variable is used to access the contents of each
	// element in the Depth() array.
	for _, interval := range help.Depth() {

		// This code is used to retrieve two candles with a given resolution from a spot exchange. The purpose of the migrate,
		// err := e.GetCandles() line is to make a request to the spot exchange using the BaseUnit, QuoteUnit, Limit, and
		// Resolution parameters provided. The if err != nil { return err } line is used to check if there was an error with
		// the request and return that error if necessary.
		migrate, err := e.GetCandles(context.Background(), &pbspot.GetRequestCandles{BaseUnit: param[0].GetBaseUnit(), QuoteUnit: param[1].GetQuoteUnit(), Limit: 2, Resolution: interval})
		if err != nil {
			return err
		}

		// This code is used to publish a message to an exchange on a specific topic. The message is "migrate" and the topic is
		// "trade/candles:interval". The purpose of this code is to send a message to the exchange,
		// action based on the message. The if statement is used to check for any errors that may occur during the publishing
		// of the message. If an error is encountered, it will be returned.
		if err := e.Context.Publish(migrate, "exchange", fmt.Sprintf("trade/candles:%v", interval)); err != nil {
			return err
		}
	}

	return nil
}

// setOrder - This function is used to set an order in the database. It takes in a pointer to a pbspot.Order which contains the
// order's details, and inserts the data into the 'orders' table. It then returns the id of the newly created order and any potential errors.
func (e *Service) setOrder(order *pbspot.Order) (id int64, err error) {

	if err := e.Context.Db.QueryRow("insert into orders (assigning, base_unit, quote_unit, price, value, quantity, user_id, type) values ($1, $2, $3, $4, $5, $6, $7, $8) returning id", order.GetAssigning(), order.GetBaseUnit(), order.GetQuoteUnit(), order.GetPrice(), order.GetQuantity(), order.GetValue(), order.GetUserId(), order.GetType()).Scan(&id); err != nil {
		return id, err
	}

	return id, nil
}

// setAsset - This function is used to set a new asset for a given user. It takes in three parameters - a string symbol to identify
// the asset, an int64 userId to identify the user, and a boolean error indicating whether an error should be returned if
// the asset already exists. The function checks if the asset already exists in the database, and if it does not exist,
// it inserts it into the database. If the error boolean is true, it will return an error if the asset already exists. If
// the error boolean is false, it will return no error regardless of the asset's existence.
func (e *Service) setAsset(symbol string, userId int64, error bool) error {

	// The purpose of this code is to query the database for a specific asset with a given symbol and userId. The query is
	// then stored in a row variable and an error is checked for. If there is an error, it will be returned. Finally, the
	// row is closed when the code is finished.
	row, err := e.Context.Db.Query(`select id from assets where symbol = $1 and user_id = $2`, symbol, userId)
	if err != nil {
		return err
	}
	defer row.Close()

	// The code is used to check if the row is valid. The '!' operator is used to check if the row is not valid. If the row
	// is not valid, the code will execute.
	if !row.Next() {

		// This code is inserting values into a database table called "assets" with the specific columns "user_id" and
		// "symbol". The purpose of this code is to save the values of userId and symbol into the table for future reference.
		if _, err = e.Context.Db.Exec("insert into assets (user_id, symbol) values ($1, $2)", userId, symbol); err != nil {
			return err
		}

		return nil
	}

	// The purpose of this code is to return an error status to indicate that a fiat asset has already been generated. The
	// code uses the status.Error() function to return an error with a specific status code (700991) and an error message
	// ("the fiat asset has already been generated").
	if error {
		return status.Error(700991, "the fiat asset has already been generated")
	}

	return nil
}

// getAsset - This function is used to determine if an asset exists for a given user. It takes two arguments, a symbol and a userId,
// and returns a boolean indicating whether the asset exists. The function executes a SQL query to the database to
// check if the asset exists, and then returns the boolean result.
func (e *Service) getAsset(symbol string, userId int64) (exist bool) {
	_ = e.Context.Db.QueryRow("select exists(select balance as balance from assets where symbol = $1 and user_id = $2)::bool", symbol, userId).Scan(&exist)
	return exist
}

// setBalance - This function is used to update the balance of a user in a database. Depending on the cross parameter, either the
// balance is increased (pbspot.Balance_PLUS) or decreased (pbspot.Balance_MINUS) by a given quantity. The balance is
// updated in the assets table of the database, using a query. Finally, an error is returned if an error occurred during the update.
func (e *Service) setBalance(symbol string, userId int64, quantity float64, cross pbspot.Balance) error {

	switch cross {
	case pbspot.Balance_PLUS:

		// The code above is an if statement that is used to update the balance of an asset with a given symbol and user_id in
		// a database. The statement executes an update query, passing in the values of symbol, quantity, and userId as
		// parameters to the query. If the query fails to execute, the if statement will return an error.
		if _, err := e.Context.Db.Exec("update assets set balance = balance + $2 where symbol = $1 and user_id = $3;", symbol, quantity, userId); err != nil {
			return err
		}
		break
	case pbspot.Balance_MINUS:

		// This code is used to update the balance of a user's assets in a database. The code updates the user's balance by
		// subtracting the quantity given. The values being used to update the balance are stored in variables, and are passed
		// into the code as parameters ($1, $2, and $3). The code also checks for errors and returns an error if one is found.
		if _, err := e.Context.Db.Exec("update assets set balance = balance - $2 where symbol = $1 and user_id = $3;", symbol, quantity, userId); err != nil {
			return err
		}
		break
	}

	return nil
}

// setTransaction - The purpose of this code is to set the transaction of a service. It checks if a transaction exists, then generates a
// unique identifier if it does not. It then inserts the transaction information into a database table, and sets the
// chain associated with the transaction. It then sets the chain.RPC and chainId values to empty strings and zero
// respectively. It then checks if the transaction has a parent, and if it does, it updates the allocation and status of
// the transaction and its parent in the database. Finally, it returns the transaction if the operation was successful, or nil if not.
func (e *Service) setTransaction(transaction *pbspot.Transaction) (*pbspot.Transaction, error) {

	// The purpose of the code snippet is to declare two variables, exist and err. exist is of type bool and err is of type
	// error. This is typically used when programming to indicate the existence of an error or to store the value of an error.
	var (
		exist bool
		err   error
	)

	// The purpose of this code is to check if the transaction exists in e.getTransactionExist(transaction.GetHash()). If
	// the transaction does not exist (i.e. !exist is true) then the code will execute the statements that follow.
	if _ = e.Context.Db.QueryRow("select exists(select id from transactions where hash = $1)::bool", transaction.GetHash()).Scan(&exist); !exist {

		// This code is used to generate a unique identifier (in this case a UUID) for a transaction if it doesn't already have
		// one. This UUID can be used to identify the transaction uniquely and ensure that it is not a duplicate of another transaction.
		if len(transaction.GetHash()) == 0 {
			transaction.Hash = uuid.NewV1().String()
		}

		// This code is a SQL query to insert transaction information into a database table called "transactions". It is
		// assigning values to each of the 13 columns in the table, and then returning the id, CreateAt, and Status columns in
		// the same row. It is then using the Scan() function to assign the returned values to the transaction object.
		if err := e.Context.Db.QueryRow(`insert into transactions (symbol, hash, value, fees, confirmation, "to", block, chain_id, user_id, assignment, type, platform, protocol, allocation, parent) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) returning id, create_at, status;`,
			transaction.GetSymbol(),
			transaction.GetHash(),
			transaction.GetValue(),
			transaction.GetFees(),
			transaction.GetConfirmation(),
			transaction.GetTo(),
			transaction.GetBlock(),
			transaction.GetChainId(),
			transaction.GetUserId(),
			transaction.GetAssignment(),
			transaction.GetType(),
			transaction.GetPlatform(),
			transaction.GetProtocol(),
			transaction.GetAllocation(),
			transaction.GetParent(),
		).Scan(&transaction.Id, &transaction.CreateAt, &transaction.Status); err != nil {
			return transaction, err
		}

		// This code is getting the chain associated with the transaction. The first line is getting the chain, and the second
		// line checks if there has been an error. If there is an error, the code returns nil and the error.
		transaction.Chain, err = e.getChain(transaction.GetChainId(), false)
		if err != nil {
			return transaction, err
		}

		// This code sets the Chain.Rpc and ChainId values of the transaction variable to empty strings and zero respectively.
		// This is likely used to reset the transaction variable to its default values.
		transaction.Chain.Rpc, transaction.ChainId = "", 0

		return transaction, nil
	} else {

		// This code is used to update the status of a transaction in a database. The first `if` statement checks if the
		// transaction has a parent. If it does, the code will execute two `Exec` commands in order to update the allocation
		// and status of the transaction and its parent in the database.
		if _ = e.Context.Db.QueryRow("select exists(select id from transactions where hash = $1 and allocation = $2)::bool", transaction.GetHash(), pbspot.Allocation_INTERNAL).Scan(&exist); exist {

			// This code is used to update a transaction in a database. It sets the assignment to DEPOSIT and the status to
			// PENDING by using the hash of the transaction as an identifier. The if statement is used to check for any errors
			// that may occur while executing the query. If an error occurs, the transaction is returned without any changes.
			if _, err := e.Context.Db.Exec("update transactions set assignment = $1, status = $2 where hash = $3;", pbspot.Assignment_DEPOSIT, pbspot.Status_PENDING, transaction.GetHash()); err != nil {
				return transaction, nil
			}

		}
	}

	return nil, nil
}

// getFees - This function is used to calculate the fees for a given symbol and whether the user is a maker or not. It first
// queries the currency table in the database to get the fees_trade and fees_discount information for the given symbol.
// It then sets the fees to the fees_trade value. If the user is a maker, it subtracts the fees_discount from the
// fees_trade to get the correct fees value. Finally, it returns the fees.
func (e *Service) getFees(symbol string, maker bool) (fees float64) {

	var (
		discount float64
	)

	// This code is checking the database for a currency with the given symbol and assigning the fees_trade and
	// fees_discount values to the fees and discount variables. If an error occurs while querying the database, the code
	// returns the fees variable.
	if err := e.Context.Db.QueryRow("select fees_trade, fees_discount from currencies where symbol = $1", symbol).Scan(&fees, &discount); err != nil {
		return fees
	}

	// This code is used to subtract a discount from the fees. If the maker variable is true, then the fees are adjusted by
	// subtracting the discount from them.
	if maker {
		fees = decimal.New(fees).Sub(discount).Float()
	}

	return fees
}

// getSum - This function is used to calculate the balance and fees for a trade. It takes the symbol for the currency, the value
// of the trade, and a boolean to indicate if the trade is a maker or taker. It then queries the database to get the fees
// and discounts associated with the trade, and applies the discount if the trade is a maker. Finally, it calculates the
// fees from the current amount and returns the new balance including fees.
func (e *Service) getSum(symbol string, value float64, maker bool) (balance, fees float64) {

	var (
		discount float64
	)

	// This code is used to query a database for a particular record associated with the given symbol. It then scans the
	// result and stores the values of the fees_trade and fees_discount columns in the variables fees and discount
	// respectively. If an error occurs during the query, it returns the balance and fees variables.
	if err := e.Context.Db.QueryRow("select fees_trade, fees_discount from currencies where symbol = $1", symbol).Scan(&fees, &discount); err != nil {
		return balance, fees
	}

	// This code is checking if the variable "maker" is true, and if it is, it is subtracting the value of "discount" from
	// "fees" and storing the result in "fees" as a float. This is likely being done to calculate a discounted fee for a maker order.
	if maker {
		fees = decimal.New(fees).Sub(discount).Float()
	}

	// This code is used to calculate the final value of a given value after subtracting fees. The two return values
	// represent the actual value after subtracting fees and the rounded value after subtracting fees.
	return value - (value - (value - decimal.New(value).Mul(fees).Float()/100)), value - (value - decimal.New(value).Mul(fees).Float()/100)
}

// getAddress - This function is used to get the address associated with a userId, symbol, platform and protocol. It does this by
// querying the assets and wallets tables in the database for a matching userId, symbol, platform, and protocol, and
// returns the address associated with the query if one is found.
func (e *Service) getAddress(userId int64, symbol string, platform pbspot.Platform, protocol pbspot.Protocol) (address string) {

	// This statement is used to query a database to get an address associated with a user, platform, protocol, and symbol.
	// The purpose of using `coalesce` is to return a blank string if the address is null. The purpose of using `QueryRow`
	// is to limit the query to a single row. The purpose of using `Scan` is to store the result of the query into the `address` variable.
	_ = e.Context.Db.QueryRow("select coalesce(w.address, '') from assets a inner join wallets w on w.platform = $3 and w.protocol = $4 and w.symbol = a.symbol and w.user_id = a.user_id where a.symbol = $1 and a.user_id = $2", symbol, userId, platform, protocol).Scan(&address)
	return address
}

// getEntropy - This function is used to retrieve the entropy (a random string of characters) associated with a specific user account
// from a database. It takes in a user ID as an argument and returns the associated entropy and an error if one occurs.
// It first queries the database to check if the user ID and status (true) match an account in the database. If it does,
// it returns the associated entropy. Otherwise, it returns an error.
func (e *Service) getEntropy(userId int64) (entropy []byte, err error) {

	// This code is attempting to retrieve a value from the database. The specific value is entropy from a row in the
	// accounts table where the id is equal to the userId and the status is true. If there is an error, the code returns the
	// entropy value and the error.
	if err := e.Context.Db.QueryRow("select entropy from accounts where id = $1 and status = $2", userId, true).Scan(&entropy); err != nil {
		return entropy, err
	}

	return entropy, nil
}

// getQuantity - This function is used to calculate the quantity of a financial asset based on its price and whether it is a
// cross-trade or not. The function takes in the assigning (buy or sell), the quantity, the price, and a boolean value to
// check if it is a cross-trade. If it is a cross-trade, the function will divide the quantity by the price. Otherwise,
// it will multiply the quantity by the price. The function then returns the calculated quantity.
func (e *Service) getQuantity(assigning pbspot.Assigning, quantity, price float64, cross bool) float64 {

	if cross {

		// The purpose of this code is to calculate the quantity of an item by dividing it by its price. This switch statement
		// checks the assigning value to make sure it is set to "BUY", and then uses the decimal.New() method to divide the
		// quantity by the price and convert it to a float.
		switch assigning {
		case pbspot.Assigning_BUY:
			quantity = decimal.New(quantity).Div(price).Float()
		}

		return quantity

	} else {

		// This switch statement is used to determine the quantity of a purchase. In this case, if the assigning variable is
		// set to pbspot.Assigning_BUY, then the quantity will be multiplied by the price to determine the total cost of the
		// purchase.
		switch assigning {
		case pbspot.Assigning_BUY:
			quantity = decimal.New(quantity).Mul(price).Float()
		}

		return quantity
	}
}

// getVolume - This function is used to get the total volume of pending orders for a given base and quote currency and assign. The
// function uses a database query to get the sum of the values from the orders table with the given parameters and then
// stores the result in the variable 'volume'. The function then returns the volume variable.
func (e *Service) getVolume(base, quote string, assign pbspot.Assigning) (volume float64) {

	//The purpose of the code is to query a database for the sum of values in the orders table where the base_unit,
	//quote_unit, assigning, and status all match the given parameters. The value is then scanned into the variable volume.
	_ = e.Context.Db.QueryRow("select coalesce(sum(value), 0.00) from orders where base_unit = $1 and quote_unit = $2 and assigning = $3 and status = $4", base, quote, assign, pbspot.Status_PENDING).Scan(&volume)
	return volume
}

// getOrder - This function is used to retrieve an order from a database by its ID. It takes an int64 (id) as a parameter and
// returns a pointer to a "pbspot.Order" type. It uses the "QueryRow" method of the database to scan the selected row
// into the "order" variable and then returns the pointer to the order.
func (e *Service) getOrder(id int64) *pbspot.Order {

	var (
		order pbspot.Order
	)

	// This code is used to query a database for a single row of data matching the specified criteria (in this case, the "id
	// = $1" condition) and then assign the returned values to the specified variables (in this case, the fields of the
	// "order" struct). This allows the program to retrieve data from the database and store it in a convenient and organized format.
	_ = e.Context.Db.QueryRow("select id, value, quantity, price, assigning, user_id, base_unit, quote_unit, status, create_at from orders where id = $1", id).Scan(&order.Id, &order.Value, &order.Quantity, &order.Price, &order.Assigning, &order.UserId, &order.BaseUnit, &order.QuoteUnit, &order.Status, &order.CreateAt)
	return &order
}

// getBalance - This function is used to query the balance of a user's assets by symbol. It takes a symbol and userID as parameters
// and queries the assets table in the database for the balance associated with that symbol and userID, then returns the balance.
func (e *Service) getBalance(symbol string, userId int64) (balance float64) {

	// This line of code is used to retrieve the balance from the assets table in a database. It takes in two parameters
	// (symbol and userId) and uses them to query the database. The result is then stored in the variable balance.
	_ = e.Context.Db.QueryRow("select balance as balance from assets where symbol = $1 and user_id = $2", symbol, userId).Scan(&balance)
	return balance
}

// getRange - This function is used to retrieve the minimum and maximum trade value of a given currency symbol from a database and
// to check if a given value is within the range. If the given value is within the range, it will return the min and max
// trade values, as well as a boolean value indicating whether the given value is within the range.
func (e *Service) getRange(symbol string, value float64) (min, max float64, ok bool) {

	// This if statement is used to query a database for a row containing the min_trade and max_trade columns for the
	// currency with the symbol given as an argument. If the query is successful, the values for min_trade and max_trade are
	// stored in the variables min and max. If the query fails, an error is returned and the function returns min, max, and ok.
	if err := e.Context.Db.QueryRow("select min_trade, max_trade from currencies where symbol = $1", symbol).Scan(&min, &max); err != nil {
		return min, max, ok
	}

	// This statement is checking to see if a given value is within a minimum and maximum range. If the value is between the
	// min and max values, then the function returns the min and max values, along with a boolean value of true.
	if value >= min && value <= max {
		return min, max, true
	}

	return min, max, ok
}

// getUnit - This function is used to get a unit from a database based on a given symbol. It queries the database for a row that
// contains the given symbol as either the base_unit or the quote_unit, and scans the row for the id, price, base_unit,
// quote_unit, and status of the unit. If successful, it returns the response and nil for the error, otherwise it returns
// an empty response and the error.
func (e *Service) getUnit(symbol string) (*pbspot.Pair, error) {

	var (
		response pbspot.Pair
	)

	// This code is part of a function which queries a database for a row that matches the given symbol. The if statement is
	// used to scan the row for the requested values and return the response. If an error occurs during the scanning
	// process, the function returns the response and the error.
	if err := e.Context.Db.QueryRow(`select id, price, base_unit, quote_unit, status from pairs where base_unit = $1 or quote_unit = $1`, symbol).Scan(&response.Id, &response.Price, &response.BaseUnit, &response.QuoteUnit, &response.Status); err != nil {
		return &response, err
	}

	return &response, nil
}

// getCurrency - This function is used to retrieve currency information from a database. It takes a currency symbol and a status
// boolean as arguments. It then queries the database to retrieve information about the currency and stores it in the
// 'response' variable. It then checks for the existence of the currency icon and stores the result in the 'icon' field
// of the 'response' variable. Finally, it returns the currency and an error value, if any.
func (e *Service) getCurrency(symbol string, status bool) (*pbspot.Currency, error) {

	var (
		response pbspot.Currency
		maps     []string
		storage  []string
		chains   []byte
	)

	// The purpose of this code is to append an item to a list of maps if a certain condition is met. In this case, if the
	// "status" variable is true, a string will be appended to the list of maps.
	if status {
		maps = append(maps, fmt.Sprintf("and status = %v", true))
	}

	// This code is performing a query of a database table called "currencies" and scanning the results into a response
	// object. The query is using the symbol parameter to filter the results and strings.Join(maps, " ") to join any
	// additional parameters. If the query fails, an error is returned.
	if err := e.Context.Db.QueryRow(fmt.Sprintf("select id, name, symbol, min_withdraw, max_withdraw, min_trade, max_trade, fees_trade, fees_discount, fees_charges, fees_costs, marker, status, type, create_at, chains from currencies where symbol = '%v' %s", symbol, strings.Join(maps, " "))).Scan(
		&response.Id,
		&response.Name,
		&response.Symbol,
		&response.MinWithdraw,
		&response.MaxWithdraw,
		&response.MinTrade,
		&response.MaxTrade,
		&response.FeesTrade,
		&response.FeesDiscount,
		&response.FeesCharges,
		&response.FeesCosts,
		&response.Marker,
		&response.Status,
		&response.Type,
		&response.CreateAt,
		&chains,
	); err != nil {
		return &response, err
	}

	// The purpose of the code is to add a string to a storage slice. The string is made up of elements from the
	// e.Context.StoragePath, the word "static", the word "icon", and a string created from the response.GetSymbol()
	// function. The ... in the code indicates that the elements of the slice are being "unpacked" into to append() call.
	storage = append(storage, []string{e.Context.StoragePath, "static", "icon", fmt.Sprintf("%v.png", response.GetSymbol())}...)

	// This statement is checking to see if a file at the given filepath exists. If it does, then the response.Icon will be
	// set to true. This statement is used in order to prevent the creation of unnecessary files.
	if _, err := os.Stat(filepath.Join(storage...)); !errors.Is(err, os.ErrNotExist) {
		response.Icon = true
	}

	// The purpose of this code is to unmarshal a json object into a response object. This is done using the
	// json.Unmarshal() function. The function takes the json object (chains) and a reference to the response.ChainsIds
	// object. If there is an error, it will be returned with the error context.
	if err := json.Unmarshal(chains, &response.ChainsIds); err != nil {
		return &response, e.Context.Error(err)
	}

	return &response, nil
}

// getContract - This function retrieves a contract from the database based on a given symbol and chain ID. It then sets the fields of
// a pbspot.Contract struct with the values from the database and returns it. Any errors encountered while retrieving the
// contract are returned as the second value.
func (e *Service) getContract(symbol string, chainId int64) (*pbspot.Contract, error) {

	var (
		contract pbspot.Contract
	)

	// This code is checking the database for a contract with the specified symbol and chain ID and then storing the results
	// of the query in a contract struct. If the query fails, to err is returned.
	if err := e.Context.Db.QueryRow(`select id, address, fees, protocol, decimals from contracts where symbol = $1 and chain_id = $2`, symbol, chainId).Scan(&contract.Id, &contract.Address, &contract.Fees, &contract.Protocol, &contract.Decimals); err != nil {
		return &contract, err
	}

	return &contract, nil
}

// getContractById - This function is used to retrieve a contract from a database by its ID. It queries the database for the contract with
// the specified ID, and then scans the query result into the fields of the pbspot.Contract struct. The function then
// returns the contract and any errors that may have occurred.
func (e *Service) getContractById(id int64) (*pbspot.Contract, error) {

	var (
		contract pbspot.Contract
	)

	// This code is a SQL query used to retrieve data from a database. The purpose of the query is to select data from the
	// contracts and chains tables for a specific contract with a given ID. The query will return information such as the
	// symbol, chain ID, address, fees withdraw, protocol, decimals, and platform of the contract. The query uses the Scan()
	// method to store the retrieved data in the contract variable. The if statement is used to check for errors and return
	// the contract along with an error if one occurs.
	if err := e.Context.Db.QueryRow(`select c.id, c.symbol, c.chain_id, c.address, c.fees, c.protocol, c.decimals, n.platform from contracts c inner join chains n on n.id = c.chain_id where c.id = $1`, id).Scan(&contract.Id, &contract.Symbol, &contract.ChainId, &contract.Address, &contract.Fees, &contract.Protocol, &contract.Decimals, &contract.Platform); err != nil {
		return &contract, err
	}

	return &contract, nil
}

// getChain - This function is used to query a row from a database table "chains" with the given id and status. It then scans the
// row into a pbspot.Chain struct. If there is an error, it returns the struct with an error. Otherwise, it returns the
// struct with no error.
func (e *Service) getChain(id int64, status bool) (*pbspot.Chain, error) {

	var (
		chain pbspot.Chain
		maps  []string
	)

	// The purpose of this code is to add the string "and status = true" to the maps slice, if the status variable is set to true.
	if status {
		maps = append(maps, fmt.Sprintf("and status = %v", true))
	}

	// This code is used to query a database for a row of data which matches the given id. The query is built by joining the
	// strings in the maps array and is passed to the QueryRow method. The data is then scanned into the chain object and
	// returned. If there is an error, it will be returned instead.
	if err := e.Context.Db.QueryRow(fmt.Sprintf("select id, name, rpc, block, network, explorer_link, platform, confirmation, time_withdraw, fees, tag, parent_symbol, decimals, status from chains where id = %[1]d %[2]s", id, strings.Join(maps, " "))).Scan(
		&chain.Id,
		&chain.Name,
		&chain.Rpc,
		&chain.Block,
		&chain.Network,
		&chain.ExplorerLink,
		&chain.Platform,
		&chain.Confirmation,
		&chain.TimeWithdraw,
		&chain.Fees,
		&chain.Tag,
		&chain.ParentSymbol,
		&chain.Decimals,
		&chain.Status,
	); err != nil {
		return &chain, errors.New("chain not found or chain network off")
	}

	return &chain, nil
}

// getPair - This function is used to get a specific pair from the database, based on the id and status passed as arguments. The
// function returns a pointer to a 'pbspot.Pair' struct and an error if any. It prepares a query to select the specified
// pair from the database, based on the given id and status. It then scans the results and stores them in the struct, and
// finally returns the struct and an error if any.
func (e *Service) getPair(id int64, status bool) (*pbspot.Pair, error) {

	var (
		chain pbspot.Pair
		maps  []string
	)

	// The purpose of this code is to append a string to a list of maps if a certain condition is true. In this case, if the
	// variable "status" is true, the string "and status = %v" with "true" as the placeholder value is added to the list of maps.
	if status {
		maps = append(maps, fmt.Sprintf("and status = %v", true))
	}

	// This code is used to query a database and retrieve information about a pair with a specified id. The query is formed
	// using the fmt.Sprintf() function, and it is a combination of a string and the id parameter. The retrieved information
	// is then assigned to the chain struct. Finally, the code returns the chain struct and an error if it fails.
	if err := e.Context.Db.QueryRow(fmt.Sprintf("select id, base_unit, quote_unit, price, base_decimal, quote_decimal, status from pairs where id = %[1]d %[2]s", id, strings.Join(maps, " "))).Scan(
		&chain.Id,
		&chain.BaseUnit,
		&chain.QuoteUnit,
		&chain.Price,
		&chain.BaseDecimal,
		&chain.QuoteDecimal,
		&chain.Status,
	); err != nil {
		return &chain, err
	}

	return &chain, nil
}

// getMarket - This function is used to get the market price for a given base and quote currency. It takes in the base, quote,
// assigning (buy/sell), and current price as parameters. It then gets the current price from the getPrice() function
// and, depending on the assigning, queries the database for either the minimum or maximum price that is greater than or
// less than the current price and is in the pending status. Finally, it returns the market price.
func (e *Service) getMarket(base, quote string, assigning pbspot.Assigning, price float64) float64 {

	var (
		ok bool
	)

	// This code is checking for the existence of a price by attempting to get it from e.getPrice(), which takes in two
	// parameters, base and quote. If the price exists (indicated by the ok return value), then it will be returned. If the
	// price does not exist (indicated by the !ok return value), then it will not be returned.
	if price, ok = e.getPrice(base, quote); !ok {
		return price
	}

	// The switch statement is used to evaluate an expression and determine which statement should be executed based on the
	// value of the expression. The switch statement assigns the expression to a variable called assigning, which is then
	// used to make the determination of which statement to execute.
	switch assigning {
	case pbspot.Assigning_BUY:

		// The purpose of this code is to query the database for the minimum price of a particular order that has a specific
		// assigning, base unit, quote unit, price, and status. The result is then stored in the variable 'price'.
		_ = e.Context.Db.QueryRow("select min(price) as price from orders where assigning = $1 and base_unit = $2 and quote_unit = $3 and price >= $4 and status = $5", pbspot.Assigning_SELL, base, quote, price, pbspot.Status_PENDING).Scan(&price)
	case pbspot.Assigning_SELL:

		// The purpose of this code is to query a database for the maximum price from orders that meet certain criteria
		// (assigning, base unit, quote unit, price and status) and scan the result into the variable "price".
		_ = e.Context.Db.QueryRow("select max(price) as price from orders where assigning = $1 and base_unit = $2 and quote_unit = $3 and price <= $4 and status = $5", pbspot.Assigning_BUY, base, quote, price, pbspot.Status_PENDING).Scan(&price)
	}

	return price
}

// getPrice - This function is used to query a database for the price of a currency pair given the base and quote units. It takes
// two parameters, base and quote, which are strings and returns a float value and a boolean. The function uses the
// QueryRow() method to execute the query, and the Scan() method to store the returned value in the price variable. If an
// error occurs, the ok boolean is returned as false, otherwise it is returned as true.
func (e *Service) getPrice(base, quote string) (price float64, ok bool) {

	// This code is used to query and retrieve a price from a database. The "if err" statement is used to check for any
	// errors that may occur during the query and retrieve process. If an error is encountered, the code will return the price and ok.
	if err := e.Context.Db.QueryRow("select price from pairs where base_unit = $1 and quote_unit = $2", base, quote).Scan(&price); err != nil {
		return price, ok
	}

	return price, true
}

// getRatio - This function is used to calculate the ratio of a given base and quote. It takes in two strings, base and quote, as
// parameters and returns a float64 representing the ratio and a boolean to indicate whether the ratio was successfully
// calculated. It uses the GetCandles function to retrieve the last 2 candles and then calculates the ratio by taking the
// difference between the first and second close prices and dividing it by the second close price.
func (e *Service) getRatio(base, quote string) (ratio float64, ok bool) {

	// This code is part of a function that is attempting to get the ratio of two different currencies. The code is
	// attempting to get two candles from the e (which is an exchange) with the given base and quote units. If an error is
	// encountered, the function will return the ratio and ok.
	migrate, err := e.GetCandles(context.Background(), &pbspot.GetRequestCandles{BaseUnit: base, QuoteUnit: quote, Limit: 2})
	if err != nil {
		return ratio, ok
	}

	// This code is checking if there are two elements in to migrate.Fields array, and if so, it is calculating the ratio
	// of the closing prices of the two elements. The ratio is calculated by subtracting the close of the first element from
	// the close of the second element, then dividing that number by the close of the second element, and then multiplying it by 100.
	if len(migrate.Fields) == 2 {
		ratio = ((migrate.Fields[0].Close - migrate.Fields[1].Close) / migrate.Fields[1].Close) * 100
	}

	return ratio, true
}

// getReserve - This function is used to get the total reserve for a given symbol, platform, and protocol from a database. It takes
// three parameters (symbol, platform, and protocol) and uses a SQL query to get the sum of the values from the reserves
// table where the symbol, platform, and protocol match the provided parameters. Finally, it returns the total reserve as a float64.
func (e *Service) getReserve(symbol string, platform pbspot.Platform, protocol pbspot.Protocol) (reserve float64) {

	// The purpose of this code is to query a database for the sum of values from a specific set of reserves (symbol,
	// platform, and protocol) and store the result in the reserve variable.
	_ = e.Context.Db.QueryRow(`select sum(value) from reserves where symbol = $1 and platform = $2 and protocol = $3`, symbol, platform, protocol).Scan(&reserve)
	return reserve
}

// setReserve - This function is used to set a reserve for a user in a database. It takes the userId, address, symbol, value,
// platform, protocol, and cross as parameters. It first checks if the reserve already exists in the database. If it
// does, it updates it depending on the value of "cross." If the reserve does not exist, it inserts a new row into the database.
func (e *Service) setReserve(userId int64, address, symbol string, value float64, platform pbspot.Platform, protocol pbspot.Protocol, cross pbspot.Balance) error {

	// This code is querying a database for a specific set of information. The code is using placeholders ($1, $2, etc.) to
	// make the query more secure by preventing SQL injection. The row variable is the result of the query, and the defer
	// statement ensures that the connection to the database is closed after the query has finished. Finally, the if
	// statement is used to check for any errors that might occur during the query.
	row, err := e.Context.Db.Query("select id from reserves where user_id = $1 and symbol = $2 and platform = $3 and protocol = $4 and address = $5", userId, symbol, platform, protocol, address)
	if err != nil {
		return err
	}
	defer row.Close()

	// The purpose of this statement is to check if there is a row available to be read from a database. If so, the code
	// within the if block will run. If not, it will skip the code within the if block.
	if row.Next() {

		switch cross {
		case pbspot.Balance_PLUS:

			// This is an example of an if statement used to update a database table. The statement checks to see if the execution
			// of an update query is successful. If the execution is unsuccessful, an error is returned. The query updates the
			// reserves table by adding a given value to the existing value for a given user and symbol, platform, protocol, and address.
			if _, err := e.Context.Db.Exec("update reserves set value = value + $6 where user_id = $1 and symbol = $2 and platform = $3 and protocol = $4 and address = $5;", userId, symbol, platform, protocol, address, value); err != nil {
				return err
			}
			break
		case pbspot.Balance_MINUS:

			// This code is used to update a database table called "reserves". It subtracts the given value from the existing
			// value in the database for the given user_id, symbol, platform, protocol and address. If an error occurs, it returns
			// the error.
			if _, err := e.Context.Db.Exec("update reserves set value = value - $6 where user_id = $1 and symbol = $2 and platform = $3 and protocol = $4 and address = $5;", userId, symbol, platform, protocol, address, value); err != nil {
				return err
			}
			break
		}

		return nil
	}

	// This code is performing an SQL query to insert data into a database table called "reserves". The data being inserted
	// consists of user_id, symbol, platform, protocol, address, and value. If there is an error in executing the query, the
	// function will return the error.
	if _, err = e.Context.Db.Exec("insert into reserves (user_id, symbol, platform, protocol, address, value) values ($1, $2, $3, $4, $5, $6)", userId, symbol, platform, protocol, address, value); err != nil {
		return err
	}

	return nil
}

// setReserveLock - This function is used to update a record in the 'reserves' table in a database. It sets the 'lock' column to 'true'
// for a record that matches the given userId, symbol, platform and protocol. This will help ensure that only one process
// can access this record in the table at any given time, allowing for concurrent access to the table without conflicts.
func (e *Service) setReserveLock(userId int64, symbol string, platform pbspot.Platform, protocol pbspot.Protocol) error {

	// This code is used to update the "lock" field in the "reserves" table in the database. The specific record to be
	// updated is identified using the userId, symbol, platform, and protocol values provided as arguments to the Exec()
	// function. If the update operation is not successful, an error is returned.
	if _, err := e.Context.Db.Exec("update reserves set lock = $5 where user_id = $1 and symbol = $2 and platform = $3 and protocol = $4;", userId, symbol, platform, protocol, true); err != nil {
		return err
	}
	return nil
}

// setReserveUnlock - The purpose of this function is to update the "lock" field of the "reserves" table in the database to false, where the
// user_id, symbol, platform, and protocol fields all match the given parameters. This is likely used to allow users to
// access the reserves for a given symbol on a given platform and protocol.
func (e *Service) setReserveUnlock(userId int64, symbol string, platform pbspot.Platform, protocol pbspot.Protocol) error {

	// This code is part of an if statement and is used to update a database table. The code is used to update a "reserves"
	// table in the database, setting the "lock" field to false where the user ID, symbol, platform, and protocol all match
	// the given parameters. If any errors occur while executing the query, the code will return an error.
	if _, err := e.Context.Db.Exec("update reserves set lock = $5 where user_id = $1 and symbol = $2 and platform = $3 and protocol = $4;", userId, symbol, platform, protocol, false); err != nil {
		return err
	}
	return nil
}

// getStatus - This function is used to check the status of two given currencies. It first checks if the currencies exist in the
// e.getCurrency function. If they do, it will return true, otherwise it will return false.
func (e *Service) getStatus(base, quote string) bool {

	// The purpose of this code is to check if an error is returned when the function "getCurrency" is called with the
	// parameters "base" and "true", and if so, return "false".
	if _, err := e.getCurrency(base, true); err != nil {
		return false
	}

	// The code snippet is checking if an error occurs when calling the function e.getCurrency() with arguments quote and
	// true. If an error occurs, it will return false.
	if _, err := e.getCurrency(quote, true); err != nil {
		return false
	}

	return true
}

// done - This function is used to mark an item with a given ID as done. The wait map is a collection of items with an
// associated boolean value indicating whether it is done or not. The function sets the value of the item with the given
// ID to true, thus marking it as done.
func (e *Service) done(id int64) {
	e.wait[id] = true
}
