"""Ed25519 (RFC 8032, section 5.1) in plain Python, for the signed record vectors.

This code is slow and not constant-time. Use it only for public test data.
"""
import functools
import hashlib

P=2**255-19
L=2**252+27742317777372353535851937790883648493
D=-121665*pow(121666,P-2,P)%P
SQRT_M1=pow(2,(P-1)//4,P)

def _sha512(data):return hashlib.sha512(data).digest()

def _add(a,b):
    """Add two points in extended coordinates (X, Y, Z, T)."""
    m=(a[1]-a[0])*(b[1]-b[0])%P;n=(a[1]+a[0])*(b[1]+b[0])%P
    c=2*a[3]*b[3]*D%P;d=2*a[2]*b[2]%P
    e,f,g,h=n-m,d-c,d+c,n+m
    return (e*f%P,g*h%P,f*g%P,e*h%P)

def _mul(scalar,point):
    out=(0,1,1,0)
    while scalar>0:
        if scalar&1:out=_add(out,point)
        point=_add(point,point);scalar>>=1
    return out

def _same(a,b):
    return (a[0]*b[2]-b[0]*a[2])%P==0 and (a[1]*b[2]-b[1]*a[2])%P==0

def _recover_x(y,sign):
    if y>=P:return None
    x2=(y*y-1)*pow(D*y*y+1,P-2,P)%P
    if x2==0:return None if sign else 0
    x=pow(x2,(P+3)//8,P)
    if (x*x-x2)%P!=0:x=x*SQRT_M1%P
    if (x*x-x2)%P!=0:return None
    if (x&1)!=sign:x=P-x
    return x

_GY=4*pow(5,P-2,P)%P
_GX=_recover_x(_GY,0)
G=(_GX,_GY,1,_GX*_GY%P)

def _compress(point):
    z=pow(point[2],P-2,P);x=point[0]*z%P;y=point[1]*z%P
    return (y|((x&1)<<255)).to_bytes(32,'little')

def _decompress(data):
    if len(data)!=32:return None
    y=int.from_bytes(data,'little');sign=y>>255;y&=(1<<255)-1
    x=_recover_x(y,sign)
    return None if x is None else (x,y,1,x*y%P)

def _expand(seed):
    h=_sha512(seed);a=int.from_bytes(h[:32],'little')
    a&=(1<<254)-8;a|=1<<254
    return a,h[32:]

def public_key(seed):
    """The 32-byte public key of a 32-byte seed."""
    return _compress(_mul(_expand(seed)[0],G))

def sign(seed,message):
    """The 64-byte signature of the message."""
    a,prefix=_expand(seed);public=_compress(_mul(a,G))
    r=int.from_bytes(_sha512(prefix+message),'little')%L;rs=_compress(_mul(r,G))
    h=int.from_bytes(_sha512(rs+public+message),'little')%L
    return rs+((r+h*a)%L).to_bytes(32,'little')

@functools.lru_cache(maxsize=None)
def verify(public,message,signature):
    """True if the signature is valid for the public key and the message."""
    if len(public)!=32 or len(signature)!=64:return False
    a=_decompress(public);r=_decompress(signature[:32])
    if a is None or r is None:return False
    s=int.from_bytes(signature[32:],'little')
    if s>=L:return False
    h=int.from_bytes(_sha512(signature[:32]+public+message),'little')%L
    return _same(_mul(s,G),_add(r,_mul(h,a)))

# RFC 8032, section 7.1, TEST 1 and TEST 2.
RFC_TESTS=(
    ('9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60',
     'd75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a','',
     'e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e06522490155'
     '5fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b'),
    ('4ccd089b28ff96da9db6c346ec114e0f5b8a319f35aba624da8cf6ed4fb8a6fb',
     '3d4017c3e843895a92b70aa74d1b7ebc9c982ccf2ec4968cc0cd55f12af4660c','72',
     '92a009a9f0d4cab8720e820b5f642540a2b27b5416503f8fb3762223ebdb69da'
     '085ac1e43e15996e458f3613d0f11d8c387b2eaeb4302aeeb00d291612bb0c00'),
)

def self_test():
    """Check this code against the RFC 8032 test vectors. Return the errors."""
    errors=[]
    for seed,public,message,signature in RFC_TESTS:
        seed,public,message,signature=(bytes.fromhex(x) for x in (seed,public,message,signature))
        if public_key(seed)!=public:errors.append('ed25519: wrong public key for an RFC 8032 test')
        if sign(seed,message)!=signature:errors.append('ed25519: wrong signature for an RFC 8032 test')
        if not verify(public,message,signature):errors.append('ed25519: an RFC 8032 signature did not verify')
        if verify(public,message+b'x',signature):errors.append('ed25519: a changed message verified')
    return errors
