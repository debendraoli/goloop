from typing import Any, Tuple, List, Union, Callable
from .client import Client
import os
from abc import *


# Convert python int to bytes of golang big.Int.
def int_to_bytes(v: int) -> bytes:
    if v == 0:
        return b''
    n_bytes = ((v + (v < 0)).bit_length() + 8) // 8
    return v.to_bytes(n_bytes, byteorder="big", signed=True)


# Convert bytes of golang big.Int to python int.
def bytes_to_int(v: bytes) -> int:
    return int.from_bytes(v, "big", signed=True)


class Message(object):
    VERSION = 0
    INVOKE = 1
    RESULT = 2
    GETVALUE = 3
    SETVALUE = 4
    CALL = 5
    EVENT = 6
    GETINFO = 7


class Status(object):
    SUCCESS = 0
    SYSTEM_FAILURE = 1


class Codec(metaclass=ABCMeta):
    @abstractmethod
    def encode(self, o: Any) -> Tuple[bytes, int]:
        pass

    @abstractmethod
    def decode(self, t: int, bs: bytes) -> Any:
        pass


class TypeTag(object):
    NIL = 0
    DICT = 1
    LIST = 2
    BYTES = 3
    STRING = 4

    CUSTOM = 10
    INT = CUSTOM + 1
    ADDRESS = CUSTOM


class ServiceManagerProxy:
    def __init__(self):
        self.__client = Client()
        self.__invoke = None
        self.__codec = None

    def connect(self, addr):
        self.__client.connect(addr)
        self.__client.send(Message.VERSION, [
            1,
            os.getpid(),
            str("python")
        ])

    def set_invoke_handler(self, invoke: Callable[[str, 'Address', 'Address', int, int, str, bytes], None]):
        self.__invoke = invoke

    def set_codec(self, codec: Codec) -> None:
        self.__codec = codec

    def decode(self, tag: int, val: bytes) -> 'Any':
        if tag == TypeTag.BYTES:
            return val
        elif tag == TypeTag.STRING:
            return val.decode('utf-8')
        elif tag == TypeTag.INT:
            return bytes_to_int(val)
        else:
            return self.__codec.decode(tag, val)

    def encode(self, o: Any) -> Tuple[bytes]:
        if o is None:
            return bytes([])
        if isinstance(o, int):
            return int_to_bytes(o)
        elif isinstance(o, str):
            return o.encode('utf-8')
        elif isinstance(o, bytes):
            return o
        else:
            v, t = self.__codec.encode(o)
            return v

    def decode_any(self, to: list) -> Any:
        tag: int = to[0]
        val: Union[bytes, dict, list] = to[1]
        if tag == TypeTag.NIL:
            return None
        elif tag == TypeTag.DICT:
            obj = {}
            for k, v in val.items():
                if isinstance(k, bytes):
                    k = k.decode('utf-8')
                obj[k] = self.decode_any(v)
            return obj
        elif tag == TypeTag.LIST:
            obj = []
            for v in val:
                obj.append(self.decode_any(v))
            return obj
        else:
            return self.decode(tag, val)

    def encode_any(self, o: Any) -> Any:
        if o is None:
            return [TypeTag.NIL, b'']
        elif isinstance(o, dict):
            m = {}
            for k, v in o.items():
                m[k] = self.encode_any(v)
            return [TypeTag.DICT, m]
        elif isinstance(o, list):
            lst = []
            for v in o:
                lst.append(self.encode_any(v))
            return [TypeTag.LIST, lst]
        elif isinstance(o, bytes):
            return [TypeTag.BYTES, o]
        elif isinstance(o, str):
            return [TypeTag.STRING, o.encode('utf-8')]
        elif isinstance(o, int):
            return [TypeTag.INT, int_to_bytes(o)]
        else:
            bs, tag = self.__codec.encode()
            return [tag, bs]

    def __handle_invoke(self, data):
        code = self.decode(TypeTag.STRING, data[0])
        _from = self.decode(TypeTag.ADDRESS, data[1])
        _to = self.decode(TypeTag.ADDRESS, data[2])
        value = self.decode(TypeTag.INT, data[3])
        limit = self.decode(TypeTag.INT, data[4])
        method = self.decode(TypeTag.STRING, data[5])
        params = data[6]

        try:
            status, step_used, result = self.__invoke(
                code, _from, _to, value, limit, method, params)

            self.__client.send(Message.RESULT, [
                status,
                self.encode(step_used),
                self.encode(result)
            ])
        except BaseException as e:
            self.__client.send(Message.RESULT, [
                Status.SYSTEM_FAILURE,
                self.encode(limit),
                None
            ])

    def loop(self):
        while True:
            msg, data = self.__client.receive()
            if msg == Message.INVOKE:
                self.__handle_invoke(data)

    def call(self, to: 'Address', value: int,
             step_limit: int, method: str,
             params: bytes) -> Tuple[int, int, bytes]:

        self.__client.send(Message.CALL, [
            self.encode(to), self.encode(value), self.encode(step_limit),
            self.encode(method), self.encode(params),
        ])

        while True:
            msg, data = self.__client.receive()
            if msg == Message.INVOKE:
                self.__handle_invoke(data)
            elif msg == Message.RESULT:
                return data[0], self.decode('int', data[1]), data[2]

    def get_value(self, key: bytes) -> bytes:
        msg, value = self.__client.send_and_receive(Message.GETVALUE, key)
        if msg != Message.GETVALUE:
            raise Exception(f'InvalidMsg({msg}) exp={Message.GETVALUE}')
        return value

    def set_value(self, key: bytes, value: bytes):
        self.__client.send(Message.SETVALUE, [key, value])

    def get_info(self) -> Any:
        msg, value = self.__client.send_and_receive(Message.GETINFO, b'')
        if msg != Message.GETINFO:
            raise Exception(f'InvalidMsg({msg}) exp={Message.GETINFO}')
        return self.decode_any(value)

    def send_event(self, idxcnt: int, event: List[Any]):
        self.__client.send(Message.EVENT, [
            idxcnt,
            map(lambda x: self.encode(x), event),
        ])
