package client

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/mondough/terrors"
)

func TestErrorSetSuite(t *testing.T) {
	suite.Run(t, new(errorSetSuite))
}

type errorSetSuite struct {
	suite.Suite
	errs    ErrorSet
	rawErrs map[string]*terrors.Error
}

func (suite *errorSetSuite) SetupTest() {
	suite.rawErrs = map[string]*terrors.Error{}
	suite.errs = nil

	err := terrors.InternalService("", "uid1", nil)
	err.Params[errUidField] = "uid1"
	err.Params[errServiceField] = "service.uid1"
	err.Params[errEndpointField] = "uid1"
	suite.errs = append(suite.errs, err)
	suite.rawErrs["uid1"] = err

	err = terrors.InternalService("", "uid2", nil)
	err.Params[errUidField] = "uid2"
	err.Params[errServiceField] = "service.uid2"
	err.Params[errEndpointField] = "uid2"
	suite.errs = append(suite.errs, err)
	suite.rawErrs["uid2"] = err

	err = terrors.InternalService("", "uid3", nil)
	err.Params[errUidField] = "uid3"
	err.Params[errServiceField] = "service.uid2" // Same service as uid2
	err.Params[errEndpointField] = "uid3"
	suite.errs = append(suite.errs, err)
	suite.rawErrs["uid3"] = err
}

func (suite *errorSetSuite) TestBasic() {
	errs := suite.errs
	err1 := suite.rawErrs["uid1"]
	err2 := suite.rawErrs["uid2"]
	err3 := suite.rawErrs["uid3"]

	suite.Assert().Len(errs, 3)
	suite.Assert().Equal(err1, errs.ForUid("uid1"))
	suite.Assert().Equal(err2, errs.ForUid("uid2"))
	suite.Assert().Equal(err3, errs.ForUid("uid3"))
	suite.Assert().True(errs.Any())
}

func (suite *errorSetSuite) TestIgnoreUid() {
	errs := suite.errs
	err2 := suite.rawErrs["uid2"]
	err3 := suite.rawErrs["uid3"]

	errs = errs.IgnoreUid("uid1")
	suite.Assert().Len(errs, 2)
	suite.Assert().Len(suite.errs, 3)
	suite.Assert().True(errs.Any())
	suite.Assert().Nil(errs.ForUid("uid1"))
	suite.Assert().Equal(err2, errs.ForUid("uid2"))
	suite.Assert().Equal(err3, errs.ForUid("uid3"))

	errs = errs.IgnoreUid("uid2", "uid3")
	suite.Assert().Empty(errs)
	suite.Assert().Len(suite.errs, 3)
	suite.Assert().False(errs.Any())
	suite.Assert().Nil(errs.ForUid("uid1"))
	suite.Assert().Nil(errs.ForUid("uid2"))
	suite.Assert().Nil(errs.ForUid("uid3"))
}

func (suite *errorSetSuite) TestIgnoreService() {
	errs := suite.errs
	err2 := suite.rawErrs["uid2"]
	err3 := suite.rawErrs["uid3"]

	errs = errs.IgnoreService("service.uid1")
	suite.Assert().Len(errs, 2)
	suite.Assert().Len(suite.errs, 3)
	suite.Assert().Nil(errs.ForUid("uid1"))
	suite.Assert().Equal(err2, errs.ForUid("uid2"))
	suite.Assert().Equal(err3, errs.ForUid("uid3"))
	suite.Assert().True(errs.Any())

	errs = errs.IgnoreService("service.uid2") // uid2 and uid3 have the same service
	suite.Assert().Empty(errs)
	suite.Assert().Len(suite.errs, 3)
	suite.Assert().Nil(errs.ForUid("uid1"))
	suite.Assert().Nil(errs.ForUid("uid2"))
	suite.Assert().Nil(errs.ForUid("uid3"))
	suite.Assert().False(errs.Any())
}

func (suite *errorSetSuite) TestIgnoreEndpoint() {
	errs := suite.errs
	err2 := suite.rawErrs["uid2"]
	err3 := suite.rawErrs["uid3"]

	errs = errs.IgnoreEndpoint("service.uid1", "uid1")
	suite.Assert().Len(errs, 2)
	suite.Assert().Len(suite.errs, 3)
	suite.Assert().True(errs.Any())
	suite.Assert().Nil(errs.ForUid("uid1"))
	suite.Assert().Equal(err2, errs.ForUid("uid2"))
	suite.Assert().Equal(err3, errs.ForUid("uid3"))

	errs = errs.IgnoreEndpoint("service.uid1", "uid10") // Doesn't exist
	suite.Assert().Len(errs, 2)
	suite.Assert().Len(suite.errs, 3)
	suite.Assert().True(errs.Any())
	suite.Assert().Nil(errs.ForUid("uid1"))
	suite.Assert().Equal(err2, errs.ForUid("uid2"))
	suite.Assert().Equal(err3, errs.ForUid("uid3"))
}

func (suite *errorSetSuite) TestIgnoreCode() {
	errs := suite.errs

	errs = errs.IgnoreCode(terrors.ErrInternalService)
	suite.Assert().Nil(errs.ForUid("uid1"))
	suite.Assert().Nil(errs.ForUid("uid2"))
	suite.Assert().Nil(errs.ForUid("uid3"))
	suite.Assert().Empty(errs)
	suite.Assert().Len(suite.errs, 3)
	suite.Assert().False(errs.Any())
}

func (suite *errorSetSuite) TestForUid() {
	errs := suite.errs
	err1 := suite.rawErrs["uid1"]
	err2 := suite.rawErrs["uid2"]
	err3 := suite.rawErrs["uid3"]

	suite.Assert().Equal(err1, errs.ForUid("uid1"))
	suite.Assert().Equal(err2, errs.ForUid("uid2"))
	suite.Assert().Equal(err3, errs.ForUid("uid3"))
}

func (suite *errorSetSuite) TestErrors() {
	suite.Assert().Equal(suite.rawErrs, suite.errs.Errors())
}

func (suite *errorSetSuite) TestMultiErrorPriority() {
	br := terrors.BadRequest("missing_param", "foo bar", nil)
	is := terrors.InternalService("something_broke", "hello world", nil)
	suite.Assert().True(higherPriority(is.Code, br.Code))
	se := terrors.New("something_else", "baz", nil)
	suite.Assert().True(higherPriority(is.Code, se.Code))
	suite.Assert().True(higherPriority(br.Code, se.Code))

	es := ErrorSet{se, is, br}
	suite.Assert().Equal(is.Code, es.Combined().(*terrors.Error).Code)
}
