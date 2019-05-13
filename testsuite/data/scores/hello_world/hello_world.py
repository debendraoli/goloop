from iconservice import *

TAG = 'HelloWorld'

class HelloWorld(IconScoreBase):
    _BALANCES = 'balances'

    def __init__(self, db: IconScoreDatabase) -> None:
        super().__init__(db)
        self._balances = DictDB(self._BALANCES, db, value_type=int)

    def on_install(self, name : str) -> None:
        super().on_install()

    def on_update(self) -> None:
        super().on_update()
    
    @external(readonly=True)
    def name(self) -> str:
        return "HelloWorld"

    @external
    def hello(self):
        Logger.info('Hello, world!', TAG)

    @external
    def helloWithName(self, name: str):
        Logger.info('Hello %s' % name,  TAG)

    @payable
    def fallback(self):
        Logger.info('fallback is called', TAG)
        self._balances[self.msg.sender] = self.msg.value

    @external
    def infiniteLoop(self):
        loopCnt = 1
        while True:
            loopCnt = loopCnt + 1
            # print("loopCnt ", loopCnt)

    @external
    @payable
    def transfer(self) -> None:
        Logger.info('Transfer!!', TAG)
        self._balances[self.msg.sender] = self.msg.value


    @external(readonly=True)
    def balanceOf(self, _owner: Address) -> str:
        print("balanceOf : ", self._balances[_owner])
        return self._balances[_owner]